//nolint:testpackage // exercises unexported registry/session internals
package mcp

import (
	"context"
	"testing"
	"time"
)

func TestSessionLogRingBuffer(t *testing.T) {
	l := &sessionLog{}

	// Fill beyond cap to trigger eviction of oldest bytes.
	big := make([]byte, maxSessionLogBytes+1024)
	for i := range big {
		big[i] = 'a'
	}

	n, err := l.Write(big)
	if err != nil || n != len(big) {
		t.Fatalf("Write returned n=%d err=%v", n, err)
	}

	if got := len(l.String()); got != maxSessionLogBytes {
		t.Errorf("buffered len = %d, want %d", got, maxSessionLogBytes)
	}

	// Subsequent small write must keep us at the cap.
	if _, err := l.Write([]byte("xyz")); err != nil {
		t.Fatalf("second Write: %v", err)
	}

	if got := len(l.String()); got != maxSessionLogBytes {
		t.Errorf("after second Write len = %d, want %d", got, maxSessionLogBytes)
	}

	if got := l.String()[maxSessionLogBytes-3:]; got != "xyz" {
		t.Errorf("tail = %q, want %q", got, "xyz")
	}
}

func TestRegisterFinishGet(t *testing.T) {
	r := &buildRegistry{sessions: make(map[string]*BuildSession)}

	s, ctx := r.Register(context.Background(), "ubuntu", "noble", "/tmp/p")
	if s.ID == "" || s.State != BuildStateRunning {
		t.Fatalf("Register returned unexpected session: %+v", s)
	}

	if ctx == nil {
		t.Fatal("Register returned nil ctx")
	}

	// Get returns a snapshot, not the same pointer.
	snap := r.Get(s.ID)
	if snap == nil || snap.ID != s.ID || snap.State != BuildStateRunning {
		t.Fatalf("Get returned %+v", snap)
	}

	if snap == s {
		t.Error("Get returned the live pointer; should be a copy")
	}

	// Done must be open before Finish, closed after.
	select {
	case <-s.Done():
		t.Fatal("Done closed before Finish")
	default:
	}

	r.Finish(s.ID, BuildStateSucceeded, "")

	select {
	case <-s.Done():
	case <-time.After(time.Second):
		t.Fatal("Done not closed after Finish")
	}

	// Finish is idempotent — second call must not panic or change state.
	r.Finish(s.ID, BuildStateFailed, "ignored")

	snap = r.Get(s.ID)
	if snap.State != BuildStateSucceeded {
		t.Errorf("state after double Finish = %s, want succeeded", snap.State)
	}
}

func TestRegisterNilParentDerivesBackground(t *testing.T) {
	r := &buildRegistry{sessions: make(map[string]*BuildSession)}

	//nolint:staticcheck // SA1012: explicitly testing nil parent fallback
	_, ctx := r.Register(nil, "", "", "")
	if ctx == nil || ctx.Err() != nil {
		t.Fatalf("ctx after nil parent: %v", ctx)
	}
}

func TestCancelMarksContext(t *testing.T) {
	r := &buildRegistry{sessions: make(map[string]*BuildSession)}

	s, ctx := r.Register(context.Background(), "", "", "")
	if !r.Cancel(s.ID) {
		t.Fatal("Cancel returned false for known ID")
	}

	if ctx.Err() == nil {
		t.Error("ctx not cancelled after Cancel")
	}

	if r.Cancel("nope") {
		t.Error("Cancel returned true for unknown ID")
	}
}

func TestUpdateContainerAndSetOutputDir(t *testing.T) {
	r := &buildRegistry{sessions: make(map[string]*BuildSession)}

	s, _ := r.Register(context.Background(), "", "", "")

	r.UpdateContainer(s.ID, "cli", "ubuntu-noble")
	r.SetOutputDir(s.ID, "/tmp/out")
	// Unknown ID = no-op, no panic.
	r.UpdateContainer("nope", "cli", "x")
	r.SetOutputDir("nope", "/x")

	snap := r.Get(s.ID)
	if !snap.InContainer || snap.ContainerRuntime != "cli" ||
		snap.ContainerImage != "ubuntu-noble" {
		t.Errorf("container fields not updated: %+v", snap)
	}

	if snap.OutputDir != "/tmp/out" {
		t.Errorf("OutputDir = %q, want /tmp/out", snap.OutputDir)
	}
}

func TestEvictionDropsOldestTerminalOnly(t *testing.T) {
	r := &buildRegistry{sessions: make(map[string]*BuildSession)}

	// One running session that must survive eviction.
	keep, _ := r.Register(context.Background(), "", "", "keep")

	// Saturate registry with terminal sessions.
	for i := range maxSessions + 10 {
		s, _ := r.Register(context.Background(), "", "", "")
		// Backdate so ordering is deterministic.
		r.sessions[s.ID].EndedAt = time.Unix(int64(i), 0)
		r.sessions[s.ID].State = BuildStateSucceeded
	}

	r.mu.Lock()
	n := len(r.sessions)
	_, kept := r.sessions[keep.ID]
	r.mu.Unlock()

	if n > maxSessions+1 {
		t.Errorf("registry size = %d, want <= %d", n, maxSessions+1)
	}

	if !kept {
		t.Error("running session was evicted")
	}
}

func TestNewBuildIDIsHex16(t *testing.T) {
	seen := map[string]bool{}

	for range 100 {
		id := newBuildID()
		if len(id) != 16 {
			t.Errorf("id length = %d, want 16", len(id))
		}

		for _, c := range id {
			if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
				t.Errorf("non-hex char in %q", id)
				break
			}
		}

		if seen[id] {
			t.Errorf("duplicate id %q", id)
		}

		seen[id] = true
	}
}
