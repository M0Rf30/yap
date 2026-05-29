package mcp

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/M0Rf30/yap/v2/pkg/errors"
)

// Tool names exposed by this file. Extracted so goconst doesn't flag
// repeated references in registration + jsonschema descriptions.
const (
	toolNameBuildLogs     = "build_logs"
	toolNameBuildWait     = "build_wait"
	toolNameBuildSummary  = "build_summary"
	toolNameListArtifacts = "list_artifacts"
)

func registerBuildExtras(srv *mcpsdk.Server) {
	registerBuildLogs(srv)
	registerBuildWait(srv)
	registerBuildSummary(srv)
	registerListArtifacts(srv)
}

// ----- build_logs ----------------------------------------------------

type buildLogsArgs struct {
	BuildID    string `json:"buildID"              jsonschema:"identifier returned by build"`
	Tail       int    `json:"tail,omitempty"       jsonschema:"return only the last N lines; 0 = all"`
	Grep       string `json:"grep,omitempty"       jsonschema:"return only lines matching this Go regexp"`
	WithLineNo bool   `json:"withLineNo,omitempty" jsonschema:"return per-line {lineno,text} structs alongside log"`
	Context    int    `json:"context,omitempty"    jsonschema:"with grep: include N lines before/after each match"`
}

// numberedLine pairs an absolute (1-based) line number with the line text.
type numberedLine struct {
	Lineno int    `json:"lineno"`
	Text   string `json:"text"`
}

type buildLogsResult struct {
	BuildID  string         `json:"buildID"`
	State    string         `json:"state"              jsonschema:"running, succeeded, failed, canceled, or unknown"`
	Lines    int            `json:"lines"              jsonschema:"line count after filtering"`
	Bytes    int            `json:"bytes"              jsonschema:"byte count after filtering"`
	Log      string         `json:"log"                jsonschema:"filtered log payload"`
	Numbered []numberedLine `json:"numbered,omitempty" jsonschema:"set when withLineNo=true"`
	//nolint:lll // jsonschema must be readable
	RegexpInvalid bool `json:"regexpInvalid,omitempty" jsonschema:"true when the grep regexp failed to compile and was ignored"`
}

func registerBuildLogs(srv *mcpsdk.Server) {
	mcpsdk.AddTool(srv, &mcpsdk.Tool{
		Name:        toolNameBuildLogs,
		Description: "Fetch captured stdout+stderr from a build session, with tail/since/grep filters.",
		Annotations: &mcpsdk.ToolAnnotations{ReadOnlyHint: true, IdempotentHint: true},
	}, func(_ context.Context, _ *mcpsdk.CallToolRequest, args buildLogsArgs,
	) (*mcpsdk.CallToolResult, buildLogsResult, error) {
		s := defaultRegistry.Get(args.BuildID)
		if s == nil {
			return nil, buildLogsResult{BuildID: args.BuildID, State: buildStateUnknown}, nil
		}

		raw := ""
		if s.Log != nil {
			raw = s.Log.String()
		}

		nl, regexpInvalid := filterLogNumbered(raw, args.Tail, args.Grep, args.Context)

		texts := make([]string, len(nl))
		for i, n := range nl {
			texts[i] = n.Text
		}

		joined := strings.Join(texts, "\n")

		out := buildLogsResult{
			BuildID:       s.ID,
			State:         string(s.State),
			Lines:         len(nl),
			Bytes:         len(joined),
			Log:           joined,
			RegexpInvalid: regexpInvalid,
		}

		if args.WithLineNo {
			out.Numbered = nl
		}

		return nil, out, nil
	})
}

// filterLogNumbered applies grep (with optional context) → tail and
// preserves the original (1-based) line number for each surviving line.
// Returns (lines, regexpInvalid). The previous `since` field was dropped
// because container build stdout has no consistent timestamp prefix to
// filter against — clients should use tail+grep instead.
func filterLogNumbered(raw string, tail int, grep string, ctxLines int) ([]numberedLine, bool) {
	if raw == "" {
		return nil, false
	}

	rawLines := strings.Split(raw, "\n")

	nl := make([]numberedLine, 0, len(rawLines))
	for i, l := range rawLines {
		nl = append(nl, numberedLine{Lineno: i + 1, Text: l})
	}

	regexpInvalid := false

	if grep != "" {
		re, err := regexp.Compile(grep)
		if err != nil {
			regexpInvalid = true
		} else {
			nl = grepWithContext(nl, re, ctxLines)
		}
	}

	if tail > 0 && len(nl) > tail {
		nl = nl[len(nl)-tail:]
	}

	return nl, regexpInvalid
}

// grepWithContext keeps lines matching re plus ctx lines before/after each
// hit. Dedup is preserved by tracking the set of kept indices.
func grepWithContext(in []numberedLine, re *regexp.Regexp, ctx int) []numberedLine {
	if ctx < 0 {
		ctx = 0
	}

	keep := make([]bool, len(in))

	for i, l := range in {
		if !re.MatchString(l.Text) {
			continue
		}

		lo := max(0, i-ctx)
		hi := min(len(in)-1, i+ctx)

		for j := lo; j <= hi; j++ {
			keep[j] = true
		}
	}

	out := make([]numberedLine, 0, len(in))

	for i, k := range keep {
		if k {
			out = append(out, in[i])
		}
	}

	return out
}

// ----- build_wait ----------------------------------------------------

type buildWaitArgs struct {
	BuildID string `json:"buildID"             jsonschema:"identifier returned by build"`
	//nolint:lll // jsonschema must be readable
	TimeoutSec int `json:"timeoutSec,omitempty" jsonschema:"max seconds to wait; 0/omitted uses the 50s server cap. Poll again when timedOut=true"`
}

type buildWaitResult struct {
	BuildID   string `json:"buildID"`
	State     string `json:"state"               jsonschema:"final state, or 'running' on timeout"`
	TimedOut  bool   `json:"timedOut,omitempty"`
	Error     string `json:"error,omitempty"`
	StartedAt string `json:"startedAt,omitempty"`
	EndedAt   string `json:"endedAt,omitempty"`
}

func registerBuildWait(srv *mcpsdk.Server) {
	mcpsdk.AddTool(srv, &mcpsdk.Tool{
		Name:        toolNameBuildWait,
		Description: "Block until a build reaches a terminal state or the timeout elapses.",
		Annotations: &mcpsdk.ToolAnnotations{ReadOnlyHint: true},
	}, func(ctx context.Context, _ *mcpsdk.CallToolRequest, args buildWaitArgs,
	) (*mcpsdk.CallToolResult, buildWaitResult, error) {
		s := defaultRegistry.Get(args.BuildID)
		if s == nil {
			return nil, buildWaitResult{BuildID: args.BuildID, State: buildStateUnknown}, nil
		}

		waitCtx, cancel := buildWaitContext(ctx, args.TimeoutSec)
		defer cancel()

		// done was captured in the copy returned by Get; channels are reference
		// types so waiting on it is safe.
		timedOut := false

		select {
		case <-s.Done():
		case <-waitCtx.Done():
			timedOut = true
		}

		final := defaultRegistry.Get(args.BuildID)
		if final == nil {
			final = s
		}

		out := buildWaitResult{
			BuildID:   final.ID,
			State:     string(final.State),
			TimedOut:  timedOut,
			Error:     final.Err,
			StartedAt: final.StartedAt.Format(time.RFC3339),
		}

		if !final.EndedAt.IsZero() {
			out.EndedAt = final.EndedAt.Format(time.RFC3339)
		}

		return nil, out, nil
	})
}

// maxBuildWaitSec caps how long build_wait blocks the JSON-RPC response.
// MCP clients enforce their own per-request timeout (commonly ~60s) and
// abort with "-32001 Request timed out" if the server holds the call
// longer. We always return before that fires; callers poll again when the
// result reports timedOut=true.
const maxBuildWaitSec = 50

// buildWaitContext returns a context that is cancelled either when parent
// is cancelled or when the effective deadline elapses. timeoutSec <= 0
// requests "wait forever", but we always clamp to maxBuildWaitSec so the
// tool call returns before the MCP client's request timeout. The returned
// cancel func is always non-nil.
func buildWaitContext(parent context.Context, timeoutSec int) (context.Context, context.CancelFunc) {
	if timeoutSec <= 0 || timeoutSec > maxBuildWaitSec {
		timeoutSec = maxBuildWaitSec
	}

	return context.WithTimeout(parent, time.Duration(timeoutSec)*time.Second)
}

// ----- build_summary -------------------------------------------------

type buildSummaryArgs struct {
	BuildID string `json:"buildID" jsonschema:"identifier returned by build"`
}

type buildSummaryResult struct {
	BuildID       string   `json:"buildID"`
	State         string   `json:"state"               jsonschema:"running, succeeded, failed, canceled, or unknown"`
	DurationSec   int      `json:"durationSec,omitempty"`
	Error         string   `json:"error,omitempty"     jsonschema:"top-level error from the build pipeline"`
	LastErrorLine string   `json:"lastErrorLine,omitempty" jsonschema:"last log line matching ERROR/FAIL/fatal"`
	FailedStep    string   `json:"failedStep,omitempty" jsonschema:"phase tag from log: build/strip/package/sign/sbom"`
	ArtifactCount int      `json:"artifactCount"       jsonschema:"recognised artifacts found in output dir"`
	Hints         []string `json:"hints,omitempty"     jsonschema:"diagnostic hints inferred from log keywords"`
}

func registerBuildSummary(srv *mcpsdk.Server) {
	mcpsdk.AddTool(srv, &mcpsdk.Tool{
		Name: toolNameBuildSummary,
		Description: "One-shot terminal diagnosis: state, duration, last error line, " +
			"best-guess failed step, artifact count, and keyword-based hints. " +
			"Cheaper than fetching full logs.",
		Annotations: &mcpsdk.ToolAnnotations{ReadOnlyHint: true, IdempotentHint: true},
	}, func(_ context.Context, _ *mcpsdk.CallToolRequest, args buildSummaryArgs,
	) (*mcpsdk.CallToolResult, buildSummaryResult, error) {
		s := defaultRegistry.Get(args.BuildID)
		if s == nil {
			return nil, buildSummaryResult{BuildID: args.BuildID, State: buildStateUnknown}, nil
		}

		out := buildSummaryResult{
			BuildID: s.ID,
			State:   string(s.State),
			Error:   s.Err,
		}

		if !s.EndedAt.IsZero() {
			out.DurationSec = int(s.EndedAt.Sub(s.StartedAt).Seconds())
		} else {
			out.DurationSec = int(time.Since(s.StartedAt).Seconds())
		}

		if s.Log != nil {
			raw := s.Log.String()
			out.LastErrorLine, out.FailedStep = lastErrorLine(raw)
			out.Hints = inferHints(raw)
		}

		if dir := outputDirForSession(s); dir != "" {
			if arts, err := scanArtifacts(dir); err == nil {
				out.ArtifactCount = len(arts)
			}
		}

		return nil, out, nil
	})
}

// errorLineRE matches the typical noise we want to extract from build logs.
//
//nolint:gochecknoglobals
var errorLineRE = regexp.MustCompile(
	`(?i)\b(error|fail(ed|ure)?|fatal|undefined reference|cannot execute|panic|abort)\b`)

// phaseLineRE picks up yap's own phase markers from the log so we can label
// where the build broke.
//
//nolint:gochecknoglobals
var phaseLineRE = regexp.MustCompile(`(?i)(prepare|build|strip|package|signing|sbom|fakeroot)`)

// lastErrorLine returns the last log line matching errorLineRE plus a
// best-guess phase tag derived from the most recent phase marker preceding
// the error.
func lastErrorLine(raw string) (lastErr, phase string) {
	if raw == "" {
		return "", ""
	}

	for l := range strings.SplitSeq(raw, "\n") {
		if errorLineRE.MatchString(l) {
			lastErr = l
		}

		if m := phaseLineRE.FindString(l); m != "" {
			phase = strings.ToLower(m)
		}
	}

	return lastErr, phase
}

// inferHints scans the log for well-known failure signatures and returns
// human-actionable hints. Conservative on purpose — only emit a hint when the
// keyword is unambiguous.
func inferHints(raw string) []string {
	if raw == "" {
		return nil
	}

	type rule struct {
		needle string
		hint   string
	}

	rules := []rule{
		{"unsupported target architecture",
			"targetArch not in archTargetTable — pass aarch64/x86_64 or alias arm64/amd64."},
		{"cannot execute binary file",
			"foreign-arch binary executed on host; check cross-toolchain pre-install."},
		{"undefined reference to",
			"linker error — likely missing -dev package or wrong cross toolchain."},
		{"Perl is required",
			"install perl-interpreter (RPM) or perl (DEB) in makedepends."},
		{"No such file or directory",
			"missing source/dep at expected path; verify makedepends + source array."},
		{"GPG signing failed",
			"check signKey resolution chain (CLI flag → env → yap.json → ~/.config/yap/keys)."},
		{"no space left on device",
			"build dir filled the disk; run `yap zap` or set buildDir to a larger volume."},
	}

	lower := strings.ToLower(raw)

	var hints []string

	for _, r := range rules {
		if strings.Contains(lower, strings.ToLower(r.needle)) {
			hints = append(hints, r.hint)
		}
	}

	return hints
}

// ----- list_artifacts ------------------------------------------------

type listArtifactsArgs struct {
	BuildID string `json:"buildID,omitempty" jsonschema:"build session whose output dir to scan"`
	Path    string `json:"path,omitempty"    jsonschema:"yap.json/PKGBUILD/dir to scan (used when buildID empty)"`
}

type artifactInfo struct {
	Path         string `json:"path"`
	Format       string `json:"format"        jsonschema:"deb, rpm, apk, pkg, or unknown"`
	SizeBytes    int64  `json:"sizeBytes"`
	HasCycloneDX bool   `json:"hasCycloneDX"`
	HasSPDX      bool   `json:"hasSPDX"`
	HasSig       bool   `json:"hasSig"`
}

type listArtifactsResult struct {
	OutputDir string         `json:"outputDir"`
	Artifacts []artifactInfo `json:"artifacts"`
}

func registerListArtifacts(srv *mcpsdk.Server) {
	mcpsdk.AddTool(srv, &mcpsdk.Tool{
		Name:        toolNameListArtifacts,
		Description: "List built package artifacts (.deb/.rpm/.apk/.pkg.tar.*) plus sibling SBOM/sig presence.",
		Annotations: &mcpsdk.ToolAnnotations{ReadOnlyHint: true},
	}, func(_ context.Context, _ *mcpsdk.CallToolRequest, args listArtifactsArgs,
	) (*mcpsdk.CallToolResult, listArtifactsResult, error) {
		outputDir, err := resolveArtifactsDir(args)
		if err != nil {
			return nil, listArtifactsResult{}, err
		}

		arts, err := scanArtifacts(outputDir)
		if err != nil {
			return nil, listArtifactsResult{OutputDir: outputDir}, err
		}

		return nil, listArtifactsResult{OutputDir: outputDir, Artifacts: arts}, nil
	})
}

// resolveArtifactsDir picks the output directory to scan. Preference order:
//  1. args.BuildID → session.OutputDir (cached at build time).
//  2. args.Path     → falls back to the user-provided path itself.
func resolveArtifactsDir(args listArtifactsArgs) (string, error) {
	switch {
	case args.BuildID != "":
		s := defaultRegistry.Get(args.BuildID)
		if s == nil {
			return "", errors.New(errors.ErrTypeValidation, "unknown buildID").
				WithContext("buildID", args.BuildID).
				WithOperation(toolNameListArtifacts)
		}

		return outputDirForSession(s), nil
	case args.Path != "":
		abs, err := resolveProjectDir(args.Path)
		if err != nil {
			return "", err
		}

		return abs, nil
	default:
		return "", errors.New(errors.ErrTypeValidation,
			"either buildID or path is required").
			WithOperation(toolNameListArtifacts)
	}
}

// outputDirForSession returns the cached output directory for a session,
// falling back to the session's project path when the build never finished
// far enough to record one.
func outputDirForSession(s *BuildSession) string {
	if s == nil {
		return ""
	}

	if s.OutputDir != "" {
		if abs, err := filepath.Abs(s.OutputDir); err == nil {
			return abs
		}

		return s.OutputDir
	}

	return s.Path
}

// scanArtifacts walks dir non-recursively and returns a sorted slice of
// recognised package files plus sibling metadata flags.
func scanArtifacts(dir string) ([]artifactInfo, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}

		return nil, errors.Wrap(err, errors.ErrTypeFileSystem,
			"read output dir").WithContext("dir", dir)
	}

	out := make([]artifactInfo, 0, len(entries))

	for _, e := range entries {
		if e.IsDir() {
			continue
		}

		name := e.Name()
		format := detectArtifactFormat(name)

		if format == artifactFormatUnknown {
			continue
		}

		full := filepath.Join(dir, name)

		st, err := e.Info()
		if err != nil {
			continue
		}

		info := artifactInfo{
			Path:      full,
			Format:    format,
			SizeBytes: st.Size(),
		}

		if _, err := os.Stat(full + ".cdx.json"); err == nil {
			info.HasCycloneDX = true
		}

		if _, err := os.Stat(full + ".spdx.json"); err == nil {
			info.HasSPDX = true
		}

		for _, suf := range []string{".asc", ".sig"} {
			if _, err := os.Stat(full + suf); err == nil {
				info.HasSig = true
				break
			}
		}

		out = append(out, info)
	}

	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })

	return out, nil
}
