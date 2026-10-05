// SPDX-FileCopyrightText: 2021-2026 Gianluca Boiano
// SPDX-License-Identifier: GPL-3.0-only

package shell

import (
	"io"
	"sync"
)

// maxCapturedOutput bounds how much script output is retained for error
// reporting. Only the most recent bytes are kept, which is where the failure
// cause normally lives.
const maxCapturedOutput = 256 * 1024

// scriptOutput is a goroutine-safe io.Writer that forwards every write to dst
// and keeps a bounded tail of the output for error reporting.
//
// mvdan/sh may write to Stdout/Stderr concurrently (background jobs,
// pipelines), so all state, including the downstream writer, is guarded by a
// single mutex.
type scriptOutput struct {
	mu        sync.Mutex
	dst       io.Writer
	tail      []byte
	truncated bool
}

// newScriptOutput returns a scriptOutput forwarding to dst.
func newScriptOutput(dst io.Writer) *scriptOutput {
	return &scriptOutput{dst: dst}
}

// Write implements io.Writer.
func (s *scriptOutput) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.capture(p)

	return s.dst.Write(p)
}

// capture appends p to the bounded tail buffer.
func (s *scriptOutput) capture(p []byte) {
	if len(p) >= maxCapturedOutput {
		s.tail = append(s.tail[:0], p[len(p)-maxCapturedOutput:]...)
		s.truncated = true

		return
	}

	s.tail = append(s.tail, p...)

	// Compact lazily so appends stay amortised O(1).
	if len(s.tail) > 2*maxCapturedOutput {
		n := copy(s.tail, s.tail[len(s.tail)-maxCapturedOutput:])
		s.tail = s.tail[:n]
		s.truncated = true
	}
}

// String returns the retained tail of the output (at most maxCapturedOutput
// bytes). If older output was discarded, the leading partial line is dropped.
func (s *scriptOutput) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()

	tail := s.tail
	if len(tail) > maxCapturedOutput {
		tail = tail[len(tail)-maxCapturedOutput:]
		s.truncated = true
	}

	if s.truncated {
		for i, c := range tail {
			if c == '\n' {
				return string(tail[i+1:])
			}
		}
	}

	return string(tail)
}
