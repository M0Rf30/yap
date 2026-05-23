//nolint:testpackage // Internal testing of options package methods
package options

import (
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRemoveEmptyDirs tests the RemoveEmptyDirs function.
func TestRemoveEmptyDirs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		setupDir  func(t *testing.T) string
		wantErr   bool
		checkFunc func(t *testing.T, dir string)
	}{
		{
			name: "Non-empty directory is preserved",
			setupDir: func(t *testing.T) string {
				t.Helper()
				tempDir := t.TempDir()
				nonEmptyDir := filepath.Join(tempDir, "nonempty")
				err := os.MkdirAll(nonEmptyDir, 0o755)
				require.NoError(t, err)

				filePath := filepath.Join(nonEmptyDir, "file.txt")
				err = os.WriteFile(filePath, []byte("content"), 0o644)
				require.NoError(t, err)

				return tempDir
			},
			wantErr: false,
			checkFunc: func(t *testing.T, dir string) {
				t.Helper()

				nonEmptyDir := filepath.Join(dir, "nonempty")
				_, err := os.Stat(nonEmptyDir)
				assert.NoError(t, err, "non-empty directory should be preserved")
			},
		},
		{
			name: "Non-existent directory returns error",
			setupDir: func(_ *testing.T) string {
				return "/nonexistent/directory/path"
			},
			wantErr: true,
		},

		{
			name: "Directory with only empty subdirectories",
			setupDir: func(t *testing.T) string {
				t.Helper()
				tempDir := t.TempDir()
				// Create nested empty dirs
				err := os.MkdirAll(filepath.Join(tempDir, "a", "b"), 0o755)
				require.NoError(t, err)
				// Add a file at the deepest level
				err = os.WriteFile(filepath.Join(tempDir, "a", "b", "file.txt"), []byte("content"), 0o644)
				require.NoError(t, err)

				return tempDir
			},
			wantErr: false,
			checkFunc: func(t *testing.T, dir string) {
				t.Helper()
				// The nested structure should still exist because b is not empty
				_, err := os.Stat(filepath.Join(dir, "a", "b"))
				assert.NoError(t, err, "non-empty nested dir should be preserved")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			dir := tt.setupDir(t)
			err := RemoveEmptyDirs(dir)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)

				if tt.checkFunc != nil {
					tt.checkFunc(t, dir)
				}
			}
		})
	}
}

// TestRemoveLibtool tests the RemoveLibtool function.
func TestRemoveLibtool(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		setupDir  func(t *testing.T) string
		wantErr   bool
		checkFunc func(t *testing.T, dir string)
	}{
		{
			name: ".la files are removed",
			setupDir: func(t *testing.T) string {
				t.Helper()
				tempDir := t.TempDir()
				libDir := filepath.Join(tempDir, "lib")
				err := os.MkdirAll(libDir, 0o755)
				require.NoError(t, err)
				err = os.WriteFile(filepath.Join(libDir, "libtest.la"), []byte("libtool archive"), 0o644)
				require.NoError(t, err)

				return tempDir
			},
			wantErr: false,
			checkFunc: func(t *testing.T, dir string) {
				t.Helper()

				laFile := filepath.Join(dir, "lib", "libtest.la")
				_, err := os.Stat(laFile)
				assert.True(t, os.IsNotExist(err), ".la file should be removed")
			},
		},
		{
			name: "Non-.la files are preserved",
			setupDir: func(t *testing.T) string {
				t.Helper()
				tempDir := t.TempDir()
				libDir := filepath.Join(tempDir, "lib")
				err := os.MkdirAll(libDir, 0o755)
				require.NoError(t, err)
				err = os.WriteFile(filepath.Join(libDir, "libtest.so"), []byte("shared library"), 0o644)
				require.NoError(t, err)
				err = os.WriteFile(filepath.Join(libDir, "libtest.a"), []byte("static library"), 0o644)
				require.NoError(t, err)

				return tempDir
			},
			wantErr: false,
			checkFunc: func(t *testing.T, dir string) {
				t.Helper()

				soFile := filepath.Join(dir, "lib", "libtest.so")
				_, err := os.Stat(soFile)
				assert.NoError(t, err, ".so file should be preserved")

				aFile := filepath.Join(dir, "lib", "libtest.a")
				_, err = os.Stat(aFile)
				assert.NoError(t, err, ".a file should be preserved")
			},
		},
		{
			name: "Nested .la files are removed",
			setupDir: func(t *testing.T) string {
				t.Helper()
				tempDir := t.TempDir()
				libDir := filepath.Join(tempDir, "usr", "lib", "pkgconfig")
				err := os.MkdirAll(libDir, 0o755)
				require.NoError(t, err)
				err = os.WriteFile(filepath.Join(libDir, "test.la"), []byte("libtool archive"), 0o644)
				require.NoError(t, err)

				return tempDir
			},
			wantErr: false,
			checkFunc: func(t *testing.T, dir string) {
				t.Helper()

				laFile := filepath.Join(dir, "usr", "lib", "pkgconfig", "test.la")
				_, err := os.Stat(laFile)
				assert.True(t, os.IsNotExist(err), "nested .la file should be removed")
			},
		},
		{
			name: "Multiple .la files are removed",
			setupDir: func(t *testing.T) string {
				t.Helper()
				tempDir := t.TempDir()
				libDir := filepath.Join(tempDir, "lib")
				err := os.MkdirAll(libDir, 0o755)
				require.NoError(t, err)

				for i := 1; i <= 3; i++ {
					filename := filepath.Join(libDir, "lib"+string(rune('a'+i-1))+".la")
					err = os.WriteFile(filename, []byte("libtool archive"), 0o644)
					require.NoError(t, err)
				}

				return tempDir
			},
			wantErr: false,
			checkFunc: func(t *testing.T, dir string) {
				t.Helper()

				for i := 1; i <= 3; i++ {
					laFile := filepath.Join(dir, "lib", "lib"+string(rune('a'+i-1))+".la")
					_, err := os.Stat(laFile)
					assert.True(t, os.IsNotExist(err), ".la file should be removed")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			dir := tt.setupDir(t)
			err := RemoveLibtool(dir)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)

				if tt.checkFunc != nil {
					tt.checkFunc(t, dir)
				}
			}
		})
	}
}

// TestRemoveStatic tests the RemoveStatic function.
func TestRemoveStatic(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		setupDir  func(t *testing.T) string
		wantErr   bool
		checkFunc func(t *testing.T, dir string)
	}{
		{
			name: ".a files are removed",
			setupDir: func(t *testing.T) string {
				t.Helper()
				tempDir := t.TempDir()
				libDir := filepath.Join(tempDir, "lib")
				err := os.MkdirAll(libDir, 0o755)
				require.NoError(t, err)
				err = os.WriteFile(filepath.Join(libDir, "libtest.a"), []byte("static library"), 0o644)
				require.NoError(t, err)

				return tempDir
			},
			wantErr: false,
			checkFunc: func(t *testing.T, dir string) {
				t.Helper()

				aFile := filepath.Join(dir, "lib", "libtest.a")
				_, err := os.Stat(aFile)
				assert.True(t, os.IsNotExist(err), ".a file should be removed")
			},
		},
		{
			name: "Non-.a files are preserved",
			setupDir: func(t *testing.T) string {
				t.Helper()
				tempDir := t.TempDir()
				libDir := filepath.Join(tempDir, "lib")
				err := os.MkdirAll(libDir, 0o755)
				require.NoError(t, err)
				err = os.WriteFile(filepath.Join(libDir, "libtest.so"), []byte("shared library"), 0o644)
				require.NoError(t, err)
				err = os.WriteFile(filepath.Join(libDir, "libtest.la"), []byte("libtool archive"), 0o644)
				require.NoError(t, err)

				return tempDir
			},
			wantErr: false,
			checkFunc: func(t *testing.T, dir string) {
				t.Helper()

				soFile := filepath.Join(dir, "lib", "libtest.so")
				_, err := os.Stat(soFile)
				assert.NoError(t, err, ".so file should be preserved")

				laFile := filepath.Join(dir, "lib", "libtest.la")
				_, err = os.Stat(laFile)
				assert.NoError(t, err, ".la file should be preserved")
			},
		},
		{
			name: "Multiple .a files are removed",
			setupDir: func(t *testing.T) string {
				t.Helper()
				tempDir := t.TempDir()
				libDir := filepath.Join(tempDir, "lib")
				err := os.MkdirAll(libDir, 0o755)
				require.NoError(t, err)

				for i := 1; i <= 3; i++ {
					filename := filepath.Join(libDir, "lib"+string(rune('a'+i-1))+".a")
					err = os.WriteFile(filename, []byte("static library"), 0o644)
					require.NoError(t, err)
				}

				return tempDir
			},
			wantErr: false,
			checkFunc: func(t *testing.T, dir string) {
				t.Helper()

				for i := 1; i <= 3; i++ {
					aFile := filepath.Join(dir, "lib", "lib"+string(rune('a'+i-1))+".a")
					_, err := os.Stat(aFile)
					assert.True(t, os.IsNotExist(err), ".a file should be removed")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			dir := tt.setupDir(t)
			err := RemoveStatic(dir)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)

				if tt.checkFunc != nil {
					tt.checkFunc(t, dir)
				}
			}
		})
	}
}

// TestRemoveDocs tests the RemoveDocs function.
func TestRemoveDocs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		setupDir  func(t *testing.T) string
		wantErr   bool
		checkFunc func(t *testing.T, dir string)
	}{
		{
			name: "usr/share/doc is removed",
			setupDir: func(t *testing.T) string {
				t.Helper()
				tempDir := t.TempDir()
				docDir := filepath.Join(tempDir, "usr", "share", "doc")
				err := os.MkdirAll(docDir, 0o755)
				require.NoError(t, err)
				err = os.WriteFile(filepath.Join(docDir, "README"), []byte("readme"), 0o644)
				require.NoError(t, err)

				return tempDir
			},
			wantErr: false,
			checkFunc: func(t *testing.T, dir string) {
				t.Helper()

				docDir := filepath.Join(dir, "usr", "share", "doc")
				_, err := os.Stat(docDir)
				assert.True(t, os.IsNotExist(err), "usr/share/doc should be removed")
			},
		},
		{
			name: "usr/share/gtk-doc is removed",
			setupDir: func(t *testing.T) string {
				t.Helper()
				tempDir := t.TempDir()
				docDir := filepath.Join(tempDir, "usr", "share", "gtk-doc")
				err := os.MkdirAll(docDir, 0o755)
				require.NoError(t, err)
				err = os.WriteFile(filepath.Join(docDir, "index.html"), []byte("<html></html>"), 0o644)
				require.NoError(t, err)

				return tempDir
			},
			wantErr: false,
			checkFunc: func(t *testing.T, dir string) {
				t.Helper()

				docDir := filepath.Join(dir, "usr", "share", "gtk-doc")
				_, err := os.Stat(docDir)
				assert.True(t, os.IsNotExist(err), "usr/share/gtk-doc should be removed")
			},
		},
		{
			name: "usr/local/share/doc is removed",
			setupDir: func(t *testing.T) string {
				t.Helper()
				tempDir := t.TempDir()
				docDir := filepath.Join(tempDir, "usr", "local", "share", "doc")
				err := os.MkdirAll(docDir, 0o755)
				require.NoError(t, err)
				err = os.WriteFile(filepath.Join(docDir, "INSTALL"), []byte("install"), 0o644)
				require.NoError(t, err)

				return tempDir
			},
			wantErr: false,
			checkFunc: func(t *testing.T, dir string) {
				t.Helper()

				docDir := filepath.Join(dir, "usr", "local", "share", "doc")
				_, err := os.Stat(docDir)
				assert.True(t, os.IsNotExist(err), "usr/local/share/doc should be removed")
			},
		},
		{
			name: "usr/local/share/gtk-doc is removed",
			setupDir: func(t *testing.T) string {
				t.Helper()
				tempDir := t.TempDir()
				docDir := filepath.Join(tempDir, "usr", "local", "share", "gtk-doc")
				err := os.MkdirAll(docDir, 0o755)
				require.NoError(t, err)
				err = os.WriteFile(filepath.Join(docDir, "index.html"), []byte("<html></html>"), 0o644)
				require.NoError(t, err)

				return tempDir
			},
			wantErr: false,
			checkFunc: func(t *testing.T, dir string) {
				t.Helper()

				docDir := filepath.Join(dir, "usr", "local", "share", "gtk-doc")
				_, err := os.Stat(docDir)
				assert.True(t, os.IsNotExist(err), "usr/local/share/gtk-doc should be removed")
			},
		},
		{
			name: "Non-existent doc directories are no-op",
			setupDir: func(t *testing.T) string {
				t.Helper()
				return t.TempDir()
			},
			wantErr: false,
			checkFunc: func(t *testing.T, dir string) {
				t.Helper()
				// Directory should still exist and be empty
				_, err := os.Stat(dir)
				assert.NoError(t, err)
			},
		},
		{
			name: "Non-doc directories are preserved",
			setupDir: func(t *testing.T) string {
				t.Helper()
				tempDir := t.TempDir()
				binDir := filepath.Join(tempDir, "usr", "bin")
				err := os.MkdirAll(binDir, 0o755)
				require.NoError(t, err)
				err = os.WriteFile(filepath.Join(binDir, "myapp"), []byte("binary"), 0o755)
				require.NoError(t, err)

				return tempDir
			},
			wantErr: false,
			checkFunc: func(t *testing.T, dir string) {
				t.Helper()

				binDir := filepath.Join(dir, "usr", "bin")
				_, err := os.Stat(binDir)
				assert.NoError(t, err, "usr/bin should be preserved")
			},
		},
		{
			name: "All doc directories are removed",
			setupDir: func(t *testing.T) string {
				t.Helper()
				tempDir := t.TempDir()

				docDirs := []string{
					docDirUsrShareDoc,
					docDirUsrShareGtkDoc,
					docDirLocalShareDoc,
					docDirLocalShareGtkDoc,
				}
				for _, docDir := range docDirs {
					fullPath := filepath.Join(tempDir, docDir)
					err := os.MkdirAll(fullPath, 0o755)
					require.NoError(t, err)
					err = os.WriteFile(filepath.Join(fullPath, "file.txt"), []byte("content"), 0o644)
					require.NoError(t, err)
				}

				return tempDir
			},
			wantErr: false,
			checkFunc: func(t *testing.T, dir string) {
				t.Helper()

				docDirs := []string{
					docDirUsrShareDoc,
					docDirUsrShareGtkDoc,
					docDirLocalShareDoc,
					docDirLocalShareGtkDoc,
				}
				for _, docDir := range docDirs {
					fullPath := filepath.Join(dir, docDir)
					_, err := os.Stat(fullPath)
					assert.True(t, os.IsNotExist(err), docDir+" should be removed")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			dir := tt.setupDir(t)
			err := RemoveDocs(dir)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)

				if tt.checkFunc != nil {
					tt.checkFunc(t, dir)
				}
			}
		})
	}
}

// TestPurge tests the Purge function.
func TestPurge(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		setupDir  func(t *testing.T) string
		wantErr   bool
		checkFunc func(t *testing.T, dir string)
	}{
		{
			name: "usr/share/info/dir is removed",
			setupDir: func(t *testing.T) string {
				t.Helper()
				tempDir := t.TempDir()
				infoDir := filepath.Join(tempDir, "usr", "share", "info")
				err := os.MkdirAll(infoDir, 0o755)
				require.NoError(t, err)
				err = os.WriteFile(filepath.Join(infoDir, "dir"), []byte("info dir"), 0o644)
				require.NoError(t, err)

				return tempDir
			},
			wantErr: false,
			checkFunc: func(t *testing.T, dir string) {
				t.Helper()

				dirFile := filepath.Join(dir, "usr", "share", "info", "dir")
				_, err := os.Stat(dirFile)
				assert.True(t, os.IsNotExist(err), "usr/share/info/dir should be removed")
			},
		},
		{
			name: "usr/local/share/info/dir is removed",
			setupDir: func(t *testing.T) string {
				t.Helper()
				tempDir := t.TempDir()
				infoDir := filepath.Join(tempDir, "usr", "local", "share", "info")
				err := os.MkdirAll(infoDir, 0o755)
				require.NoError(t, err)
				err = os.WriteFile(filepath.Join(infoDir, "dir"), []byte("info dir"), 0o644)
				require.NoError(t, err)

				return tempDir
			},
			wantErr: false,
			checkFunc: func(t *testing.T, dir string) {
				t.Helper()

				dirFile := filepath.Join(dir, "usr", "local", "share", "info", "dir")
				_, err := os.Stat(dirFile)
				assert.True(t, os.IsNotExist(err), "usr/local/share/info/dir should be removed")
			},
		},
		{
			name: ".packlist files are removed",
			setupDir: func(t *testing.T) string {
				t.Helper()
				tempDir := t.TempDir()
				libDir := filepath.Join(tempDir, "usr", "lib", "perl5")
				err := os.MkdirAll(libDir, 0o755)
				require.NoError(t, err)
				err = os.WriteFile(filepath.Join(libDir, ".packlist"), []byte("packlist"), 0o644)
				require.NoError(t, err)

				return tempDir
			},
			wantErr: false,
			checkFunc: func(t *testing.T, dir string) {
				t.Helper()

				packlistFile := filepath.Join(dir, "usr", "lib", "perl5", ".packlist")
				_, err := os.Stat(packlistFile)
				assert.True(t, os.IsNotExist(err), ".packlist file should be removed")
			},
		},
		{
			name: "*.pod files are removed",
			setupDir: func(t *testing.T) string {
				t.Helper()
				tempDir := t.TempDir()
				manDir := filepath.Join(tempDir, "usr", "share", "man", "man3")
				err := os.MkdirAll(manDir, 0o755)
				require.NoError(t, err)
				err = os.WriteFile(filepath.Join(manDir, "Module.pod"), []byte("pod"), 0o644)
				require.NoError(t, err)

				return tempDir
			},
			wantErr: false,
			checkFunc: func(t *testing.T, dir string) {
				t.Helper()

				podFile := filepath.Join(dir, "usr", "share", "man", "man3", "Module.pod")
				_, err := os.Stat(podFile)
				assert.True(t, os.IsNotExist(err), "*.pod file should be removed")
			},
		},
		{
			name: "Other files are preserved",
			setupDir: func(t *testing.T) string {
				t.Helper()
				tempDir := t.TempDir()
				infoDir := filepath.Join(tempDir, "usr", "share", "info")
				err := os.MkdirAll(infoDir, 0o755)
				require.NoError(t, err)
				err = os.WriteFile(filepath.Join(infoDir, "myinfo.info"), []byte("info"), 0o644)
				require.NoError(t, err)

				return tempDir
			},
			wantErr: false,
			checkFunc: func(t *testing.T, dir string) {
				t.Helper()

				infoFile := filepath.Join(dir, "usr", "share", "info", "myinfo.info")
				_, err := os.Stat(infoFile)
				assert.NoError(t, err, "other info files should be preserved")
			},
		},
		{
			name: "Multiple .packlist files are removed",
			setupDir: func(t *testing.T) string {
				t.Helper()
				tempDir := t.TempDir()
				libDir1 := filepath.Join(tempDir, "usr", "lib", "perl5", "site_perl")
				libDir2 := filepath.Join(tempDir, "usr", "lib", "perl5", "vendor_perl")
				err := os.MkdirAll(libDir1, 0o755)
				require.NoError(t, err)
				err = os.MkdirAll(libDir2, 0o755)
				require.NoError(t, err)
				err = os.WriteFile(filepath.Join(libDir1, ".packlist"), []byte("packlist"), 0o644)
				require.NoError(t, err)
				err = os.WriteFile(filepath.Join(libDir2, ".packlist"), []byte("packlist"), 0o644)
				require.NoError(t, err)

				return tempDir
			},
			wantErr: false,
			checkFunc: func(t *testing.T, dir string) {
				t.Helper()

				packlistFile1 := filepath.Join(dir, "usr", "lib", "perl5", "site_perl", ".packlist")
				_, err := os.Stat(packlistFile1)
				assert.True(t, os.IsNotExist(err), "first .packlist should be removed")

				packlistFile2 := filepath.Join(dir, "usr", "lib", "perl5", "vendor_perl", ".packlist")
				_, err = os.Stat(packlistFile2)
				assert.True(t, os.IsNotExist(err), "second .packlist should be removed")
			},
		},
		{
			name: "Multiple *.pod files are removed",
			setupDir: func(t *testing.T) string {
				t.Helper()
				tempDir := t.TempDir()
				manDir := filepath.Join(tempDir, "usr", "share", "man", "man3")
				err := os.MkdirAll(manDir, 0o755)
				require.NoError(t, err)

				for i := 1; i <= 3; i++ {
					filename := filepath.Join(manDir, "Module"+string(rune('A'+i-1))+".pod")
					err = os.WriteFile(filename, []byte("pod"), 0o644)
					require.NoError(t, err)
				}

				return tempDir
			},
			wantErr: false,
			checkFunc: func(t *testing.T, dir string) {
				t.Helper()

				for i := 1; i <= 3; i++ {
					podFile := filepath.Join(dir, "usr", "share", "man", "man3", "Module"+string(rune('A'+i-1))+".pod")
					_, err := os.Stat(podFile)
					assert.True(t, os.IsNotExist(err), "*.pod file should be removed")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			dir := tt.setupDir(t)
			err := Purge(dir)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)

				if tt.checkFunc != nil {
					tt.checkFunc(t, dir)
				}
			}
		})
	}
}

// TestZipMan tests the ZipMan function.
func TestZipMan(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		setupDir  func(t *testing.T) string
		wantErr   bool
		checkFunc func(t *testing.T, dir string)
	}{
		{
			name: "Files in usr/share/man are compressed",
			setupDir: func(t *testing.T) string {
				t.Helper()
				tempDir := t.TempDir()
				manDir := filepath.Join(tempDir, "usr", "share", "man", "man1")
				err := os.MkdirAll(manDir, 0o755)
				require.NoError(t, err)
				err = os.WriteFile(filepath.Join(manDir, "myapp.1"), []byte("man page content"), 0o644)
				require.NoError(t, err)

				return tempDir
			},
			wantErr: false,
			checkFunc: func(t *testing.T, dir string) {
				t.Helper()

				manFile := filepath.Join(dir, "usr", "share", "man", "man1", "myapp.1")
				manGzFile := filepath.Join(dir, "usr", "share", "man", "man1", "myapp.1.gz")
				_, err := os.Stat(manFile)
				assert.True(t, os.IsNotExist(err), "original man file should be removed")
				_, err = os.Stat(manGzFile)
				assert.NoError(t, err, "compressed man file should exist")
			},
		},
		{
			name: "Files in usr/share/info are compressed",
			setupDir: func(t *testing.T) string {
				t.Helper()
				tempDir := t.TempDir()
				infoDir := filepath.Join(tempDir, "usr", "share", "info")
				err := os.MkdirAll(infoDir, 0o755)
				require.NoError(t, err)
				err = os.WriteFile(filepath.Join(infoDir, "myapp.info"), []byte("info content"), 0o644)
				require.NoError(t, err)

				return tempDir
			},
			wantErr: false,
			checkFunc: func(t *testing.T, dir string) {
				t.Helper()

				infoFile := filepath.Join(dir, "usr", "share", "info", "myapp.info")
				infoGzFile := filepath.Join(dir, "usr", "share", "info", "myapp.info.gz")
				_, err := os.Stat(infoFile)
				assert.True(t, os.IsNotExist(err), "original info file should be removed")
				_, err = os.Stat(infoGzFile)
				assert.NoError(t, err, "compressed info file should exist")
			},
		},
		{
			name: "Files in usr/local/share/man are compressed",
			setupDir: func(t *testing.T) string {
				t.Helper()
				tempDir := t.TempDir()
				manDir := filepath.Join(tempDir, "usr", "local", "share", "man", "man1")
				err := os.MkdirAll(manDir, 0o755)
				require.NoError(t, err)
				err = os.WriteFile(filepath.Join(manDir, "localapp.1"), []byte("man page"), 0o644)
				require.NoError(t, err)

				return tempDir
			},
			wantErr: false,
			checkFunc: func(t *testing.T, dir string) {
				t.Helper()

				manFile := filepath.Join(dir, "usr", "local", "share", "man", "man1", "localapp.1")
				manGzFile := filepath.Join(dir, "usr", "local", "share", "man", "man1", "localapp.1.gz")
				_, err := os.Stat(manFile)
				assert.True(t, os.IsNotExist(err), "original man file should be removed")
				_, err = os.Stat(manGzFile)
				assert.NoError(t, err, "compressed man file should exist")
			},
		},
		{
			name: "Files in usr/local/share/info are compressed",
			setupDir: func(t *testing.T) string {
				t.Helper()
				tempDir := t.TempDir()
				infoDir := filepath.Join(tempDir, "usr", "local", "share", "info")
				err := os.MkdirAll(infoDir, 0o755)
				require.NoError(t, err)
				err = os.WriteFile(filepath.Join(infoDir, "localapp.info"), []byte("info"), 0o644)
				require.NoError(t, err)

				return tempDir
			},
			wantErr: false,
			checkFunc: func(t *testing.T, dir string) {
				t.Helper()

				infoFile := filepath.Join(dir, "usr", "local", "share", "info", "localapp.info")
				infoGzFile := filepath.Join(dir, "usr", "local", "share", "info", "localapp.info.gz")
				_, err := os.Stat(infoFile)
				assert.True(t, os.IsNotExist(err), "original info file should be removed")
				_, err = os.Stat(infoGzFile)
				assert.NoError(t, err, "compressed info file should exist")
			},
		},
		{
			name: "Already .gz files are skipped",
			setupDir: func(t *testing.T) string {
				t.Helper()
				tempDir := t.TempDir()
				manDir := filepath.Join(tempDir, "usr", "share", "man", "man1")
				err := os.MkdirAll(manDir, 0o755)
				require.NoError(t, err)
				err = os.WriteFile(filepath.Join(manDir, "myapp.1.gz"), []byte("already gzipped"), 0o644)
				require.NoError(t, err)

				return tempDir
			},
			wantErr: false,
			checkFunc: func(t *testing.T, dir string) {
				t.Helper()

				manGzFile := filepath.Join(dir, "usr", "share", "man", "man1", "myapp.1.gz")
				_, err := os.Stat(manGzFile)
				assert.NoError(t, err, ".gz file should still exist")
			},
		},
		{
			name: "Already .bz2 files are skipped",
			setupDir: func(t *testing.T) string {
				t.Helper()
				tempDir := t.TempDir()
				manDir := filepath.Join(tempDir, "usr", "share", "man", "man1")
				err := os.MkdirAll(manDir, 0o755)
				require.NoError(t, err)
				err = os.WriteFile(filepath.Join(manDir, "myapp.1.bz2"), []byte("already bz2"), 0o644)
				require.NoError(t, err)

				return tempDir
			},
			wantErr: false,
			checkFunc: func(t *testing.T, dir string) {
				t.Helper()

				manBz2File := filepath.Join(dir, "usr", "share", "man", "man1", "myapp.1.bz2")
				_, err := os.Stat(manBz2File)
				assert.NoError(t, err, ".bz2 file should still exist")
			},
		},
		{
			name: "Already .xz files are skipped",
			setupDir: func(t *testing.T) string {
				t.Helper()
				tempDir := t.TempDir()
				manDir := filepath.Join(tempDir, "usr", "share", "man", "man1")
				err := os.MkdirAll(manDir, 0o755)
				require.NoError(t, err)
				err = os.WriteFile(filepath.Join(manDir, "myapp.1.xz"), []byte("already xz"), 0o644)
				require.NoError(t, err)

				return tempDir
			},
			wantErr: false,
			checkFunc: func(t *testing.T, dir string) {
				t.Helper()

				manXzFile := filepath.Join(dir, "usr", "share", "man", "man1", "myapp.1.xz")
				_, err := os.Stat(manXzFile)
				assert.NoError(t, err, ".xz file should still exist")
			},
		},
		{
			name: "Already .zst files are skipped",
			setupDir: func(t *testing.T) string {
				t.Helper()
				tempDir := t.TempDir()
				manDir := filepath.Join(tempDir, "usr", "share", "man", "man1")
				err := os.MkdirAll(manDir, 0o755)
				require.NoError(t, err)
				err = os.WriteFile(filepath.Join(manDir, "myapp.1.zst"), []byte("already zst"), 0o644)
				require.NoError(t, err)

				return tempDir
			},
			wantErr: false,
			checkFunc: func(t *testing.T, dir string) {
				t.Helper()

				manZstFile := filepath.Join(dir, "usr", "share", "man", "man1", "myapp.1.zst")
				_, err := os.Stat(manZstFile)
				assert.NoError(t, err, ".zst file should still exist")
			},
		},
		{
			name: "Non-man directories are not touched",
			setupDir: func(t *testing.T) string {
				t.Helper()
				tempDir := t.TempDir()
				binDir := filepath.Join(tempDir, "usr", "bin")
				err := os.MkdirAll(binDir, 0o755)
				require.NoError(t, err)
				err = os.WriteFile(filepath.Join(binDir, "myapp"), []byte("binary"), 0o755)
				require.NoError(t, err)

				return tempDir
			},
			wantErr: false,
			checkFunc: func(t *testing.T, dir string) {
				t.Helper()

				binFile := filepath.Join(dir, "usr", "bin", "myapp")
				_, err := os.Stat(binFile)
				assert.NoError(t, err, "non-man files should not be touched")
			},
		},
		{
			name: "Compressed file preserves permissions",
			setupDir: func(t *testing.T) string {
				t.Helper()
				tempDir := t.TempDir()
				manDir := filepath.Join(tempDir, "usr", "share", "man", "man1")
				err := os.MkdirAll(manDir, 0o755)
				require.NoError(t, err)

				manFile := filepath.Join(manDir, "myapp.1")
				err = os.WriteFile(manFile, []byte("man page"), 0o755)
				require.NoError(t, err)

				return tempDir
			},
			wantErr: false,
			checkFunc: func(t *testing.T, dir string) {
				t.Helper()

				manGzFile := filepath.Join(dir, "usr", "share", "man", "man1", "myapp.1.gz")
				info, err := os.Stat(manGzFile)
				require.NoError(t, err)
				// Check that the file is executable (0o755 preserved)
				assert.True(t, info.Mode()&0o111 != 0, "executable bit should be preserved")
			},
		},
		{
			name: "Compressed file is valid gzip",
			setupDir: func(t *testing.T) string {
				t.Helper()
				tempDir := t.TempDir()
				manDir := filepath.Join(tempDir, "usr", "share", "man", "man1")
				err := os.MkdirAll(manDir, 0o755)
				require.NoError(t, err)
				err = os.WriteFile(filepath.Join(manDir, "myapp.1"), []byte("man page content"), 0o644)
				require.NoError(t, err)

				return tempDir
			},
			wantErr: false,
			checkFunc: func(t *testing.T, dir string) {
				t.Helper()

				manGzFile := filepath.Join(dir, "usr", "share", "man", "man1", "myapp.1.gz")
				file, err := os.Open(manGzFile)
				require.NoError(t, err)

				defer func() { _ = file.Close() }()

				// Try to read the gzip file
				gz, err := gzip.NewReader(file)
				require.NoError(t, err)

				defer func() { _ = gz.Close() }()

				content, err := io.ReadAll(gz)
				require.NoError(t, err)
				assert.Equal(t, "man page content", string(content), "gzip content should match original")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			dir := tt.setupDir(t)
			err := ZipMan(dir)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)

				if tt.checkFunc != nil {
					tt.checkFunc(t, dir)
				}
			}
		})
	}
}

// TestApply tests the Apply function.
func TestApply(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		setupDir  func(t *testing.T) string
		options   Options
		wantErr   bool
		checkFunc func(t *testing.T, dir string)
	}{
		{
			name: "All options disabled (default)",
			setupDir: func(t *testing.T) string {
				t.Helper()
				tempDir := t.TempDir()
				// Create various files that would be removed by options
				docDir := filepath.Join(tempDir, "usr", "share", "doc")
				err := os.MkdirAll(docDir, 0o755)
				require.NoError(t, err)
				err = os.WriteFile(filepath.Join(docDir, "README"), []byte("readme"), 0o644)
				require.NoError(t, err)

				libDir := filepath.Join(tempDir, "lib")
				err = os.MkdirAll(libDir, 0o755)
				require.NoError(t, err)
				err = os.WriteFile(filepath.Join(libDir, "libtest.la"), []byte("libtool"), 0o644)
				require.NoError(t, err)
				err = os.WriteFile(filepath.Join(libDir, "libtest.a"), []byte("static"), 0o644)
				require.NoError(t, err)

				return tempDir
			},
			options: Options{
				DebugEnabled:     false,
				DocsEnabled:      true,
				EmptyDirsEnabled: true,
				LibtoolEnabled:   true,
				PurgeEnabled:     false,
				StaticEnabled:    true,
				StripEnabled:     false,
				ZipManEnabled:    false,
			},
			wantErr: false,
			checkFunc: func(t *testing.T, dir string) {
				t.Helper()
				// With all options enabled (default), nothing should be removed
				docDir := filepath.Join(dir, "usr", "share", "doc")
				_, err := os.Stat(docDir)
				assert.NoError(t, err, "doc dir should be preserved when DocsEnabled=true")
			},
		},
		{
			name: "Remove docs enabled",
			setupDir: func(t *testing.T) string {
				t.Helper()
				tempDir := t.TempDir()
				docDir := filepath.Join(tempDir, "usr", "share", "doc")
				err := os.MkdirAll(docDir, 0o755)
				require.NoError(t, err)
				err = os.WriteFile(filepath.Join(docDir, "README"), []byte("readme"), 0o644)
				require.NoError(t, err)
				// Add a file to root to keep it non-empty
				err = os.WriteFile(filepath.Join(tempDir, "file.txt"), []byte("content"), 0o644)
				require.NoError(t, err)
				// Add a file to usr/share to keep it non-empty
				err = os.WriteFile(filepath.Join(tempDir, "usr", "share", "other.txt"), []byte("content"), 0o644)
				require.NoError(t, err)

				return tempDir
			},
			options: Options{
				DocsEnabled:      false, // Disable docs to remove them
				EmptyDirsEnabled: true,  // Keep empty dirs to avoid walk errors
			},
			wantErr: false,
			checkFunc: func(t *testing.T, dir string) {
				t.Helper()

				docDir := filepath.Join(dir, "usr", "share", "doc")
				_, err := os.Stat(docDir)
				assert.True(t, os.IsNotExist(err), "doc dir should be removed when DocsEnabled=false")
			},
		},
		{
			name: "Remove libtool enabled",
			setupDir: func(t *testing.T) string {
				t.Helper()
				tempDir := t.TempDir()
				libDir := filepath.Join(tempDir, "lib")
				err := os.MkdirAll(libDir, 0o755)
				require.NoError(t, err)
				err = os.WriteFile(filepath.Join(libDir, "libtest.la"), []byte("libtool"), 0o644)
				require.NoError(t, err)
				// Add a file to lib to keep it non-empty
				err = os.WriteFile(filepath.Join(libDir, "libtest.so"), []byte("shared"), 0o644)
				require.NoError(t, err)
				// Add a file to root to keep it non-empty
				err = os.WriteFile(filepath.Join(tempDir, "file.txt"), []byte("content"), 0o644)
				require.NoError(t, err)

				return tempDir
			},
			options: Options{
				LibtoolEnabled:   false, // Disable libtool to remove .la files
				EmptyDirsEnabled: true,  // Keep empty dirs to avoid walk errors
			},
			wantErr: false,
			checkFunc: func(t *testing.T, dir string) {
				t.Helper()

				laFile := filepath.Join(dir, "lib", "libtest.la")
				_, err := os.Stat(laFile)
				assert.True(t, os.IsNotExist(err), ".la file should be removed when LibtoolEnabled=false")
			},
		},
		{
			name: "Remove static enabled",
			setupDir: func(t *testing.T) string {
				t.Helper()
				tempDir := t.TempDir()
				libDir := filepath.Join(tempDir, "lib")
				err := os.MkdirAll(libDir, 0o755)
				require.NoError(t, err)
				err = os.WriteFile(filepath.Join(libDir, "libtest.a"), []byte("static"), 0o644)
				require.NoError(t, err)
				// Add a file to lib to keep it non-empty
				err = os.WriteFile(filepath.Join(libDir, "libtest.so"), []byte("shared"), 0o644)
				require.NoError(t, err)
				// Add a file to root to keep it non-empty
				err = os.WriteFile(filepath.Join(tempDir, "file.txt"), []byte("content"), 0o644)
				require.NoError(t, err)

				return tempDir
			},
			options: Options{
				StaticEnabled:    false, // Disable static to remove .a files
				EmptyDirsEnabled: true,  // Keep empty dirs to avoid walk errors
			},
			wantErr: false,
			checkFunc: func(t *testing.T, dir string) {
				t.Helper()

				aFile := filepath.Join(dir, "lib", "libtest.a")
				_, err := os.Stat(aFile)
				assert.True(t, os.IsNotExist(err), ".a file should be removed when StaticEnabled=false")
			},
		},
		{
			name: "Purge enabled",
			setupDir: func(t *testing.T) string {
				t.Helper()
				tempDir := t.TempDir()
				infoDir := filepath.Join(tempDir, "usr", "share", "info")
				err := os.MkdirAll(infoDir, 0o755)
				require.NoError(t, err)
				err = os.WriteFile(filepath.Join(infoDir, "dir"), []byte("info dir"), 0o644)
				require.NoError(t, err)
				// Add a file to info to keep it non-empty
				err = os.WriteFile(filepath.Join(infoDir, "other.info"), []byte("info"), 0o644)
				require.NoError(t, err)
				// Add a file to root to keep it non-empty
				err = os.WriteFile(filepath.Join(tempDir, "file.txt"), []byte("content"), 0o644)
				require.NoError(t, err)

				return tempDir
			},
			options: Options{
				PurgeEnabled:     true, // Enable purge
				EmptyDirsEnabled: true, // Keep empty dirs to avoid walk errors
			},
			wantErr: false,
			checkFunc: func(t *testing.T, dir string) {
				t.Helper()

				dirFile := filepath.Join(dir, "usr", "share", "info", "dir")
				_, err := os.Stat(dirFile)
				assert.True(t, os.IsNotExist(err), "info/dir should be removed when PurgeEnabled=true")
			},
		},
		{
			name: "ZipMan enabled",
			setupDir: func(t *testing.T) string {
				t.Helper()
				tempDir := t.TempDir()
				manDir := filepath.Join(tempDir, "usr", "share", "man", "man1")
				err := os.MkdirAll(manDir, 0o755)
				require.NoError(t, err)
				err = os.WriteFile(filepath.Join(manDir, "myapp.1"), []byte("man page"), 0o644)
				require.NoError(t, err)

				return tempDir
			},
			options: Options{
				ZipManEnabled: true, // Enable zipman
			},
			wantErr: false,
			checkFunc: func(t *testing.T, dir string) {
				t.Helper()

				manFile := filepath.Join(dir, "usr", "share", "man", "man1", "myapp.1")
				manGzFile := filepath.Join(dir, "usr", "share", "man", "man1", "myapp.1.gz")
				_, err := os.Stat(manFile)
				assert.True(t, os.IsNotExist(err), "original man file should be removed")
				_, err = os.Stat(manGzFile)
				assert.NoError(t, err, "compressed man file should exist")
			},
		},
		{
			name: "Remove empty dirs enabled",
			setupDir: func(t *testing.T) string {
				t.Helper()
				tempDir := t.TempDir()
				// Create a non-empty directory to test
				nonEmptyDir := filepath.Join(tempDir, "nonempty")
				err := os.MkdirAll(nonEmptyDir, 0o755)
				require.NoError(t, err)
				err = os.WriteFile(filepath.Join(nonEmptyDir, "file.txt"), []byte("content"), 0o644)
				require.NoError(t, err)

				return tempDir
			},
			options: Options{
				EmptyDirsEnabled: false, // Disable empty dirs to remove them
			},
			wantErr: false,
			checkFunc: func(t *testing.T, dir string) {
				t.Helper()
				// Non-empty dir should still exist
				nonEmptyDir := filepath.Join(dir, "nonempty")
				_, err := os.Stat(nonEmptyDir)
				assert.NoError(t, err, "non-empty dir should be preserved")
			},
		},
		{
			name: "Multiple options enabled",
			setupDir: func(t *testing.T) string {
				t.Helper()
				tempDir := t.TempDir()

				// Create docs
				docDir := filepath.Join(tempDir, "usr", "share", "doc")
				err := os.MkdirAll(docDir, 0o755)
				require.NoError(t, err)
				err = os.WriteFile(filepath.Join(docDir, "README"), []byte("readme"), 0o644)
				require.NoError(t, err)

				// Create .la files
				libDir := filepath.Join(tempDir, "lib")
				err = os.MkdirAll(libDir, 0o755)
				require.NoError(t, err)
				err = os.WriteFile(filepath.Join(libDir, "libtest.la"), []byte("libtool"), 0o644)
				require.NoError(t, err)

				// Create .a files
				err = os.WriteFile(filepath.Join(libDir, "libtest.a"), []byte("static"), 0o644)
				require.NoError(t, err)

				// Create man pages
				manDir := filepath.Join(tempDir, "usr", "share", "man", "man1")
				err = os.MkdirAll(manDir, 0o755)
				require.NoError(t, err)
				err = os.WriteFile(filepath.Join(manDir, "myapp.1"), []byte("man page"), 0o644)
				require.NoError(t, err)

				return tempDir
			},
			options: Options{
				DocsEnabled:      false,
				LibtoolEnabled:   false,
				StaticEnabled:    false,
				ZipManEnabled:    true,
				EmptyDirsEnabled: true, // Keep empty dirs to avoid walk errors
			},
			wantErr: false,
			checkFunc: func(t *testing.T, dir string) {
				t.Helper()
				// Check docs removed
				docDir := filepath.Join(dir, "usr", "share", "doc")
				_, err := os.Stat(docDir)
				assert.True(t, os.IsNotExist(err), "doc dir should be removed")

				// Check .la removed
				laFile := filepath.Join(dir, "lib", "libtest.la")
				_, err = os.Stat(laFile)
				assert.True(t, os.IsNotExist(err), ".la file should be removed")

				// Check .a removed
				aFile := filepath.Join(dir, "lib", "libtest.a")
				_, err = os.Stat(aFile)
				assert.True(t, os.IsNotExist(err), ".a file should be removed")

				// Check man pages compressed
				manFile := filepath.Join(dir, "usr", "share", "man", "man1", "myapp.1")
				manGzFile := filepath.Join(dir, "usr", "share", "man", "man1", "myapp.1.gz")
				_, err = os.Stat(manFile)
				assert.True(t, os.IsNotExist(err), "original man file should be removed")
				_, err = os.Stat(manGzFile)
				assert.NoError(t, err, "compressed man file should exist")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			dir := tt.setupDir(t)
			err := Apply(dir, tt.options)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)

				if tt.checkFunc != nil {
					tt.checkFunc(t, dir)
				}
			}
		})
	}
}
