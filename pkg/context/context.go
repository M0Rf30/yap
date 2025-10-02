// Package context provides context utilities and management for build operations.
package context

import (
	"context"
	"time"

	"github.com/M0Rf30/yap/v2/pkg/logger"
)

// contextKey is a custom type for context keys to avoid collisions.
type contextKey string

const (
	// BuildIDKey is the context key for build identifiers.
	BuildIDKey contextKey = "build_id"
	// ProjectKey is the context key for project identifiers.
	ProjectKey contextKey = "project"
	// PackageKey is the context key for package identifiers.
	PackageKey contextKey = "package"
	// DistroKey is the context key for distribution identifiers.
	DistroKey contextKey = "distro"
	// ReleaseKey is the context key for release identifiers.
	ReleaseKey contextKey = "release"
	// OperationKey is the context key for operation identifiers.
	OperationKey contextKey = "operation"
	// UserKey is the context key for user identifiers.
	UserKey contextKey = "user"
	// RequestIDKey is the context key for request identifiers.
	RequestIDKey contextKey = "request_id"
	// TraceIDKey is the context key for trace identifiers.
	TraceIDKey contextKey = "trace_id"
	// LoggerKey is the context key for logger instances.
	LoggerKey contextKey = "logger"
)

// BuildContext contains information specific to a build operation.
type BuildContext struct {
	BuildID   string            `json:"buildId"`
	Project   string            `json:"project"`
	Package   string            `json:"package"`
	Distro    string            `json:"distro"`
	Release   string            `json:"release"`
	StartTime time.Time         `json:"startTime"`
	Metadata  map[string]string `json:"metadata"`
}

// NewBuildContext creates a new build context.
func NewBuildContext(buildID, project, pkg, distro, release string) *BuildContext {
	return &BuildContext{
		BuildID:   buildID,
		Project:   project,
		Package:   pkg,
		Distro:    distro,
		Release:   release,
		StartTime: time.Now(),
		Metadata:  make(map[string]string),
	}
}

// WithBuildContext adds build context to the context.
func WithBuildContext(parent context.Context, buildCtx *BuildContext) context.Context {
	ctx := parent
	ctx = context.WithValue(ctx, BuildIDKey, buildCtx.BuildID)
	ctx = context.WithValue(ctx, ProjectKey, buildCtx.Project)
	ctx = context.WithValue(ctx, PackageKey, buildCtx.Package)
	ctx = context.WithValue(ctx, DistroKey, buildCtx.Distro)
	ctx = context.WithValue(ctx, ReleaseKey, buildCtx.Release)

	return ctx
}

// GetBuildContext extracts build context from context.
func GetBuildContext(ctx context.Context) *BuildContext {
	buildCtx := &BuildContext{
		Metadata: make(map[string]string),
	}

	if buildID, ok := ctx.Value(BuildIDKey).(string); ok {
		buildCtx.BuildID = buildID
	}

	if project, ok := ctx.Value(ProjectKey).(string); ok {
		buildCtx.Project = project
	}

	if pkg, ok := ctx.Value(PackageKey).(string); ok {
		buildCtx.Package = pkg
	}

	if distro, ok := ctx.Value(DistroKey).(string); ok {
		buildCtx.Distro = distro
	}

	if release, ok := ctx.Value(ReleaseKey).(string); ok {
		buildCtx.Release = release
	}

	return buildCtx
}

// WithTimeout creates a context with timeout and proper cleanup.
func WithTimeout(
	parent context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(parent, timeout)
}

// WithDeadline creates a context with deadline and proper cleanup.
func WithDeadline(
	parent context.Context, deadline time.Time) (context.Context, context.CancelFunc) {
	return context.WithDeadline(parent, deadline)
}

// WithCancel creates a context with cancellation.
func WithCancel(
	parent context.Context) (context.Context, context.CancelFunc) {
	return context.WithCancel(parent)
}

// WithLogger adds a logger to the context.
func WithLogger(parent context.Context, log *logger.YapLogger) context.Context {
	return context.WithValue(parent, LoggerKey, log)
}

// GetLogger retrieves logger from context, returns default if not found.
func GetLogger(ctx context.Context) *logger.YapLogger {
	if log, ok := ctx.Value(LoggerKey).(*logger.YapLogger); ok {
		return log
	}

	return logger.Logger
}

// WithTraceID adds a trace ID to the context.
func WithTraceID(parent context.Context, traceID string) context.Context {
	return context.WithValue(parent, TraceIDKey, traceID)
}

// GetTraceID retrieves trace ID from context.
func GetTraceID(ctx context.Context) string {
	if traceID, ok := ctx.Value(TraceIDKey).(string); ok {
		return traceID
	}

	return ""
}

// WithRequestID adds a request ID to the context.
func WithRequestID(parent context.Context, requestID string) context.Context {
	return context.WithValue(parent, RequestIDKey, requestID)
}

// GetRequestID retrieves request ID from context.
func GetRequestID(ctx context.Context) string {
	if requestID, ok := ctx.Value(RequestIDKey).(string); ok {
		return requestID
	}

	return ""
}

// WithOperation adds an operation name to the context.
func WithOperation(parent context.Context, operation string) context.Context {
	return context.WithValue(parent, OperationKey, operation)
}

// GetOperation retrieves operation name from context.
func GetOperation(ctx context.Context) string {
	if operation, ok := ctx.Value(OperationKey).(string); ok {
		return operation
	}

	return ""
}

// BackgroundWithTimeout creates a background context with timeout.
func BackgroundWithTimeout(timeout time.Duration) (context.Context, context.CancelFunc) {
	return WithTimeout(context.Background(), timeout)
}

// RetryWithContext retries a function with exponential backoff and context support.
//
//nolint:varnamelen // fn is a commonly used short name for function parameters
func RetryWithContext(ctx context.Context, maxRetries int, baseDelay time.Duration,
	fn func(context.Context) error,
) error {
	var lastErr error

	delay := baseDelay

	for attempt := 0; attempt <= maxRetries; attempt++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		err := fn(ctx)
		if err == nil {
			return nil
		}

		lastErr = err

		if attempt < maxRetries {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(delay):
			}

			delay *= 2 // Exponential backoff
		}
	}

	return lastErr
}
