package mcp

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"sync"
	"time"
)

// maxSessionLogBytes caps the per-session log capture so a runaway build
// can't exhaust process memory. When the buffer fills, oldest bytes are
// dropped (ring-style); callers see the most recent output, which is what
// matters for diagnosing failures.
const maxSessionLogBytes = 256 * 1024

// maxSessions caps how many terminal build sessions the registry retains.
// When exceeded, the oldest terminal sessions are evicted. Running sessions
// are never evicted. This keeps a long-running yap-mcp process bounded.
const maxSessions = 200

// sessionLog is a mutex-guarded bounded byte buffer that satisfies io.Writer.
// Older bytes are dropped once the cap is reached.
type sessionLog struct {
	mu  sync.Mutex
	buf []byte
}

// Write appends p, evicting oldest bytes when the buffer would exceed cap.
func (s *sessionLog) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.buf = append(s.buf, p...)
	if over := len(s.buf) - maxSessionLogBytes; over > 0 {
		s.buf = s.buf[over:]
	}

	return len(p), nil
}

// String returns a copy of the buffered log.
func (s *sessionLog) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()

	return string(s.buf)
}

// BuildState represents the lifecycle phase of an async yap build.
type BuildState string

// Build lifecycle states surfaced via the build_status MCP tool.
const (
	// BuildStateRunning is the initial state set when Register inserts a new
	// session into the registry.
	BuildStateRunning BuildState = "running"
	// BuildStateSucceeded is the terminal state for builds that finished
	// without error.
	BuildStateSucceeded BuildState = "succeeded"
	// BuildStateFailed is the terminal state for builds that returned a
	// non-context error from the project pipeline.
	BuildStateFailed BuildState = "failed"
	// BuildStateCanceled is the terminal state for builds whose context was
	// canceled (either by build_cancel or by server shutdown).
	BuildStateCanceled BuildState = "canceled"
)

// BuildSession is one in-flight or finished yap build invocation tracked by
// the MCP server. Clients can poll its status via build_status.
type BuildSession struct {
	ID        string
	StartedAt time.Time
	EndedAt   time.Time
	State     BuildState
	Distro    string
	Release   string
	Path      string

	// OutputDir is the resolved artifact output directory for this build,
	// cached when the build is registered so list_artifacts/build_summary
	// don't have to re-load the project on every call.
	OutputDir string

	// InContainer is true when the build was dispatched into a yap
	// container image instead of running natively in-process. Exposed in
	// build_status so clients can tell where the work is happening.
	InContainer bool
	// ContainerRuntime is the container backend ("cli" or "rootless") used
	// when InContainer is true; empty otherwise.
	ContainerRuntime string
	// ContainerImage is the resolved image tag (e.g. "ubuntu-jammy") used
	// for the dispatch; empty for native builds.
	ContainerImage string

	// Err holds the build error string when State == failed.
	Err string

	// Log captures stdout+stderr from container dispatches. Bounded by
	// maxSessionLogBytes; accessed via Log.String() in snapshots.
	Log *sessionLog

	// cancel cancels the underlying build context.
	cancel context.CancelFunc

	// done is closed by Finish so build_wait can block efficiently
	// without polling the map.
	done chan struct{}
}

// Done returns a channel closed when the build reaches a terminal state.
// Useful for build_wait. Safe to call repeatedly.
func (s *BuildSession) Done() <-chan struct{} { return s.done }

// buildRegistry is a process-wide map of buildID → BuildSession. Concurrent
// access is guarded by mu. Sessions are retained up to maxSessions; once that
// cap is reached, the oldest terminal sessions are evicted on each Register
// call (running sessions are never evicted).
type buildRegistry struct {
	mu       sync.Mutex
	sessions map[string]*BuildSession
}

//nolint:gochecknoglobals // registry is process-singleton by design
var defaultRegistry = &buildRegistry{
	sessions: make(map[string]*BuildSession),
}

// newBuildID returns a 16-hex-char random identifier (8 bytes of entropy).
func newBuildID() string {
	var b [8]byte

	_, _ = rand.Read(b[:])

	return hex.EncodeToString(b[:])
}

// Register inserts a fresh session in state "running" and returns it together
// with a context that the caller MUST pass to the build goroutine. Pass nil
// as parent to derive from context.Background.
func (r *buildRegistry) Register(parent context.Context,
	distro, release, path string,
) (*BuildSession, context.Context) {
	if parent == nil {
		parent = context.Background()
	}

	ctx, cancel := context.WithCancel(parent)

	s := &BuildSession{
		ID:        newBuildID(),
		StartedAt: time.Now().UTC(),
		State:     BuildStateRunning,
		Distro:    distro,
		Release:   release,
		Path:      path,
		Log:       &sessionLog{},
		cancel:    cancel,
		done:      make(chan struct{}),
	}

	r.mu.Lock()
	r.sessions[s.ID] = s
	r.evictOldestTerminalLocked()
	r.mu.Unlock()

	return s, ctx
}

// evictOldestTerminalLocked drops the oldest-ended terminal sessions until
// len(sessions) <= maxSessions. Running sessions are skipped. Caller must
// hold r.mu.
func (r *buildRegistry) evictOldestTerminalLocked() {
	if len(r.sessions) <= maxSessions {
		return
	}

	// Collect terminal session IDs with their EndedAt.
	type entry struct {
		id      string
		endedAt time.Time
	}

	terminals := make([]entry, 0, len(r.sessions))
	for id, s := range r.sessions {
		if s.State != BuildStateRunning {
			terminals = append(terminals, entry{id: id, endedAt: s.EndedAt})
		}
	}

	// Sort ascending by endedAt (oldest first).
	for i := 1; i < len(terminals); i++ {
		for j := i; j > 0 && terminals[j-1].endedAt.After(terminals[j].endedAt); j-- {
			terminals[j-1], terminals[j] = terminals[j], terminals[j-1]
		}
	}

	for _, e := range terminals {
		if len(r.sessions) <= maxSessions {
			break
		}

		delete(r.sessions, e.id)
	}
}

// Finish transitions a session to a terminal state. errStr is the empty
// string for successful builds.
func (r *buildRegistry) Finish(id string, state BuildState, errStr string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	s, ok := r.sessions[id]
	if !ok {
		return
	}

	if s.State != BuildStateRunning {
		// Already terminal — keep the original outcome and channel state.
		return
	}

	s.State = state
	s.Err = errStr
	s.EndedAt = time.Now().UTC()

	if s.done != nil {
		close(s.done)
	}
}

// Get returns a snapshot of the named session, or nil when unknown.
//
// Snapshot semantics: scalar fields (State, Err, EndedAt, ContainerImage…)
// are atomic at copy time. The returned Log pointer and done channel are
// shared with the live session so build_wait and Log.String() observe
// live progress — but a caller that reads State then Log.String() may see
// log content from a state that has since become terminal. This is by
// design: it lets clients tail logs of a still-running build.
func (r *buildRegistry) Get(id string) *BuildSession {
	r.mu.Lock()
	defer r.mu.Unlock()

	s, ok := r.sessions[id]
	if !ok {
		return nil
	}

	cp := *s

	return &cp
}

// UpdateContainer records that a session is running inside a container.
// Must be called before the build goroutine starts so concurrent Get
// snapshots observe the correct fields. No-op when the session is unknown.
func (r *buildRegistry) UpdateContainer(id, runtime, image string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	s, ok := r.sessions[id]
	if !ok {
		return
	}

	s.InContainer = true
	s.ContainerRuntime = runtime
	s.ContainerImage = image
}

// SetOutputDir caches the resolved artifact directory for a session so
// list_artifacts and build_summary don't re-load the project on each call.
// No-op when the session is unknown.
func (r *buildRegistry) SetOutputDir(id, dir string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if s, ok := r.sessions[id]; ok {
		s.OutputDir = dir
	}
}

// Cancel triggers cancellation on the underlying build context. Idempotent.
func (r *buildRegistry) Cancel(id string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	s, ok := r.sessions[id]
	if !ok || s.cancel == nil {
		return false
	}

	s.cancel()

	return true
}
