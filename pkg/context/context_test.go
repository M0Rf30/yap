//nolint:testpackage // Internal testing of context package methods
package context

import (
	"context"
	"testing"
	"time"

	"github.com/M0Rf30/yap/v2/pkg/logger"
)

func TestNewBuildContext(t *testing.T) {
	t.Parallel()

	buildID := "test-build-123"
	project := "test-project"
	pkg := "test-package"
	distro := "test-distro"
	release := "test-release"

	buildCtx := NewBuildContext(buildID, project, pkg, distro, release)

	if buildCtx.BuildID != buildID {
		t.Errorf("Expected BuildID %s, got %s", buildID, buildCtx.BuildID)
	}

	if buildCtx.Project != project {
		t.Errorf("Expected Project %s, got %s", project, buildCtx.Project)
	}

	if buildCtx.Package != pkg {
		t.Errorf("Expected Package %s, got %s", pkg, buildCtx.Package)
	}

	if buildCtx.Distro != distro {
		t.Errorf("Expected Distro %s, got %s", distro, buildCtx.Distro)
	}

	if buildCtx.Release != release {
		t.Errorf("Expected Release %s, got %s", release, buildCtx.Release)
	}

	if buildCtx.Metadata == nil {
		t.Error("Expected Metadata to be initialized")
	}
}

func TestWithBuildContext(t *testing.T) {
	t.Parallel()

	buildCtx := NewBuildContext("build-123", "proj", "pkg", "distro", "rel")
	ctx := WithBuildContext(context.Background(), buildCtx)

	if ctx.Value(BuildIDKey) != buildCtx.BuildID {
		t.Errorf("Expected BuildID in context to be %s", buildCtx.BuildID)
	}

	if ctx.Value(ProjectKey) != buildCtx.Project {
		t.Errorf("Expected Project in context to be %s", buildCtx.Project)
	}
}

func TestGetBuildContext(t *testing.T) {
	t.Parallel()

	originalCtx := NewBuildContext("build-123", "proj", "pkg", "distro", "rel")
	ctx := WithBuildContext(context.Background(), originalCtx)
	retrievedCtx := GetBuildContext(ctx)

	if retrievedCtx.BuildID != originalCtx.BuildID {
		t.Errorf("Expected BuildID %s, got %s", originalCtx.BuildID, retrievedCtx.BuildID)
	}

	if retrievedCtx.Project != originalCtx.Project {
		t.Errorf("Expected Project %s, got %s", originalCtx.Project, retrievedCtx.Project)
	}
}

func TestWithLogger(t *testing.T) {
	t.Parallel()

	log := logger.Logger
	ctx := WithLogger(context.Background(), log)

	retrievedLogger := GetLogger(ctx)
	if retrievedLogger != log {
		t.Error("Expected logger to match")
	}
}

func TestGetLogger_Default(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	log := GetLogger(ctx)

	if log == nil {
		t.Error("Expected default logger to be returned")
	}
}

func TestTraceID(t *testing.T) {
	t.Parallel()

	traceID := "trace-123"
	ctx := WithTraceID(context.Background(), traceID)

	retrieved := GetTraceID(ctx)
	if retrieved != traceID {
		t.Errorf("Expected TraceID %s, got %s", traceID, retrieved)
	}
}

func TestRequestID(t *testing.T) {
	t.Parallel()

	requestID := "request-123"
	ctx := WithRequestID(context.Background(), requestID)

	retrieved := GetRequestID(ctx)
	if retrieved != requestID {
		t.Errorf("Expected RequestID %s, got %s", requestID, retrieved)
	}
}

func TestOperation(t *testing.T) {
	t.Parallel()

	operation := "build"
	ctx := WithOperation(context.Background(), operation)

	retrieved := GetOperation(ctx)
	if retrieved != operation {
		t.Errorf("Expected Operation %s, got %s", operation, retrieved)
	}
}

func TestRetryWithContext(t *testing.T) {
	t.Parallel()

	attempts := 0

	err := RetryWithContext(context.Background(), 2, 10*time.Millisecond, func(_ context.Context) error {
		attempts++
		if attempts < 2 {
			return context.DeadlineExceeded
		}

		return nil
	})
	if err != nil {
		t.Fatalf("Unexpected error after retry: %v", err)
	}

	if attempts != 2 {
		t.Errorf("Expected 2 attempts, got %d", attempts)
	}
}
