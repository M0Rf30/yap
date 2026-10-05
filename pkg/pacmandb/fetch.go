// SPDX-FileCopyrightText: 2021-2026 Gianluca Boiano
// SPDX-License-Identifier: GPL-3.0-only

package pacmandb

import (
	"context"

	"github.com/M0Rf30/yap/v2/pkg/httpclient"
)

// maxPacmanDBBytes caps a downloaded <repo>.db (or .db.sig) at 256 MiB.
// Real Arch / extra / community DBs are well under 50 MB.
const maxPacmanDBBytes = 256 << 20

// downloadFile fetches url into dest via the shared httpclient (size cap,
// retry on transient failures, unique temp file + atomic rename). syncRepo's
// mirror loop handles definitive per-mirror failures.
func downloadFile(ctx context.Context, url, dest string) error {
	return httpclient.FetchToFile(ctx, url, dest, maxPacmanDBBytes)
}
