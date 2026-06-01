// Package main is the entrypoint for the yap-mcp binary — an MCP (Model
// Context Protocol) server that exposes yap's package-build capabilities to
// MCP-compatible clients (Claude Desktop, IDE agents, etc.).
//
// v1 binds the stdio transport. HTTP+SSE is planned but gated for later.
//
// Shutdown semantics: the server exits cleanly when any of these happen:
//   - stdin reaches EOF (the SDK's stdio transport Read returns and Run unblocks)
//   - SIGINT / SIGTERM is delivered
//   - parent process dies (Linux PR_SET_PDEATHSIG → SIGTERM)
//
// Logging: stdout is reserved for the JSON-RPC stream. All log output goes to
// stderr via pkg/logger. Set YAP_VERBOSE=1 for debug-level frames.
package main

import (
	"context"
	"errors"
	"os"
	"os/signal"
	"syscall"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/M0Rf30/yap/v2/pkg/buildinfo"
	"github.com/M0Rf30/yap/v2/pkg/i18n"
	"github.com/M0Rf30/yap/v2/pkg/logger"
	yapmcp "github.com/M0Rf30/yap/v2/pkg/mcp"
)

func main() {
	if os.Getenv("YAP_VERBOSE") != "" {
		logger.SetVerbose(true)
	}

	// On Linux, ask the kernel to deliver SIGTERM if our parent dies. This
	// guarantees shutdown even when stdin is held open by another descriptor.
	armParentDeathSignal()

	ctx, stop := signal.NotifyContext(context.Background(),
		os.Interrupt, syscall.SIGTERM)
	defer stop()

	logger.Info(i18n.T("logger.yap-mcp.info.yap_mcp_starting"), "version", buildinfo.Version,
		"commit", buildinfo.Commit,
		"transport", "stdio",
	)

	srv := yapmcp.NewServer()

	err := srv.Run(ctx, &mcpsdk.StdioTransport{})

	// Cancel any in-flight async builds so background goroutines unblock and
	// the runtime can exit promptly.
	stop()

	if err != nil && !isContextError(err) {
		logger.Fatal(i18n.T("logger.yap-mcp.error.yap_mcp_server_failed"), "error", err)
	}

	logger.Info(i18n.T("logger.yap-mcp.info.yap_mcp_stopped"))
}

// isContextError reports whether err is a benign context cancellation,
// emitted when SIGTERM/SIGINT triggers the signal-derived ctx to cancel.
func isContextError(err error) bool {
	return errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
}
