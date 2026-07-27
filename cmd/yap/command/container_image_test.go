package command

import (
	"errors"
	"testing"

	yapErrors "github.com/M0Rf30/yap/v2/pkg/errors"
	"github.com/M0Rf30/yap/v2/pkg/platform"
)

// TestResolveContainerImage exercises every resolution rule of
// resolveContainerImage without touching the real host /etc/os-release,
// locking in the fix for #214 where a bare `yap build ubuntu <path>`
// dispatched into the nonexistent `m0rf30/yap-ubuntu` image instead of a
// release-qualified builder image.
func TestResolveContainerImage(t *testing.T) {
	tests := []struct {
		name      string
		distro    string
		release   string
		host      platform.OSRelease
		wantImage string
		wantErr   bool
	}{
		{
			name:      "release-qualified passthrough",
			distro:    "ubuntu",
			release:   "jammy",
			host:      platform.OSRelease{ID: "fedora", Codename: "resolute"},
			wantImage: "ubuntu-jammy",
		},
		{
			name:      "release-qualified passthrough ignores host mismatch",
			distro:    "rocky",
			release:   "9",
			host:      platform.OSRelease{},
			wantImage: "rocky-9",
		},
		{
			name:      "bare alpine is a published single-name image",
			distro:    "alpine",
			host:      platform.OSRelease{ID: "ubuntu", Codename: "jammy"},
			wantImage: "alpine",
		},
		{
			name:      "bare arch is a published single-name image",
			distro:    "arch",
			host:      platform.OSRelease{ID: "ubuntu", Codename: "jammy"},
			wantImage: "arch",
		},
		{
			name:      "bare family back-filled from matching host codename",
			distro:    "ubuntu",
			host:      platform.OSRelease{ID: "ubuntu", Codename: "jammy"},
			wantImage: "ubuntu-jammy",
		},
		{
			name:    "bare family with no matching host is unresolvable",
			distro:  "fedora",
			host:    platform.OSRelease{ID: "ubuntu", Codename: "jammy"},
			wantErr: true,
		},
		{
			name:    "host matches but codename is empty is unresolvable",
			distro:  "ubuntu",
			host:    platform.OSRelease{ID: "ubuntu", Codename: ""},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			image, err := resolveContainerImage(tt.distro, tt.release, tt.host)

			if tt.wantErr {
				if err == nil {
					t.Fatalf("resolveContainerImage(%q, %q, %+v) error = nil, want error",
						tt.distro, tt.release, tt.host)
				}

				var yapErr *yapErrors.YapError
				if !errors.As(err, &yapErr) {
					t.Fatalf("resolveContainerImage(%q, %q, %+v) error type = %T, want *yapErrors.YapError",
						tt.distro, tt.release, tt.host, err)
				}

				if yapErr.Type != yapErrors.ErrTypeValidation {
					t.Errorf("resolveContainerImage(%q, %q, %+v) error type = %v, want %v",
						tt.distro, tt.release, tt.host, yapErr.Type, yapErrors.ErrTypeValidation)
				}

				return
			}

			if err != nil {
				t.Fatalf("resolveContainerImage(%q, %q, %+v) unexpected error: %v",
					tt.distro, tt.release, tt.host, err)
			}

			if image != tt.wantImage {
				t.Errorf("resolveContainerImage(%q, %q, %+v) = %q, want %q",
					tt.distro, tt.release, tt.host, image, tt.wantImage)
			}
		})
	}
}
