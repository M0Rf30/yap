// Package dnfcache module-stream support for RHEL/Rocky AppStream.
//
// RHEL 8 and derivatives ship multiple incompatible versions of the same
// package (perl, python, nodejs, ruby, postgresql, ...) as "module streams"
// in the AppStream repo. All streams are dumped into primary.xml, so a naive
// name-keyed index can land on a non-default stream and produce broken
// dependency closures (e.g. perl-5.24 installed without perl-libs-5.24,
// which lives in the modular providers we don't index).
//
// This file loads `modules.yaml(.gz|.xz)` from each repo's `repomd.xml`
// `<data type="modules">` entry and constructs:
//   - defaultStream[moduleName] = default stream (e.g. "perl" -> "5.26")
//   - allowedModularNVRA = set of NVRA strings belonging to default streams
//
// addPackage then rejects modular packages (Release contains ".module+")
// whose NVRA is not in allowedModularNVRA, leaving the non-modular variant
// (or no variant) — matching what `dnf install` would resolve to without an
// explicit `dnf module enable`.
package dnfcache

import (
	"bufio"
	"compress/gzip"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/ulikunitz/xz"
	"gopkg.in/yaml.v3"

	"github.com/M0Rf30/yap/v2/pkg/logger"
)

// moduleIndex holds the parsed module-defaults and the allowed NVRAs.
// Empty index = no filtering (non-modular repos).
type moduleIndex struct {
	defaultStream map[string]string // module -> default stream
	// blockedNVRA: NVRAs that belong to a *non-default* stream of a module
	// that has a default. These packages are filtered out (matching `dnf
	// install` without explicit `dnf module enable`). NVRAs absent from
	// this set are allowed, including default-stream NVRAs and modular
	// packages whose default stream lists no artifacts (e.g. Rocky 8.10
	// perl:5.26 where the default-stream doc has an empty artifacts list
	// but the .module+el8 RPMs still ship in AppStream repodata).
	blockedNVRA map[string]bool
}

func newModuleIndex() *moduleIndex {
	return &moduleIndex{
		defaultStream: make(map[string]string),
		blockedNVRA:   make(map[string]bool),
	}
}

// modulemdDoc is one YAML document in modules.yaml. modules.yaml is a
// multi-document YAML stream; documents have either document=modulemd
// (a stream definition with rpm artifacts) or document=modulemd-defaults
// (default stream selection for a module).
// modulemdDoc maps to either a `modulemd` (stream definition) or a
// `modulemd-defaults` (default-stream selection) YAML document. Both share
// the `stream` field key: in modulemd it's the stream name of THIS doc; in
// modulemd-defaults it's the chosen default stream for `module`.
type modulemdDoc struct {
	Document string `yaml:"document"`
	Version  int    `yaml:"version"`
	Data     struct {
		// modulemd: package name. modulemd-defaults: empty.
		Name string `yaml:"name"`
		// modulemd: this stream's name. modulemd-defaults: the default
		// stream for `module`.
		Stream    string `yaml:"stream"`
		Artifacts struct {
			RPMs []string `yaml:"rpms"`
		} `yaml:"artifacts"`
		// modulemd-defaults: target module name. modulemd: empty.
		Module string `yaml:"module"`
	} `yaml:"data"`
}

// parseModulesYAML decodes a multi-doc modules.yaml stream and merges its
// contents into idx. Malformed documents are skipped (logged at debug);
// streams that lack a default selection contribute no allowed NVRAs.
func parseModulesYAML(r io.Reader, idx *moduleIndex) {
	dec := yaml.NewDecoder(r)

	// Pass 1: collect all stream rpms keyed by name:stream; collect defaults.
	streamRPMs := make(map[string][]string) // "module:stream" -> NVRA list

	for {
		var doc modulemdDoc
		if err := dec.Decode(&doc); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			// Skip malformed documents but keep going.
			logger.Debug("dnfcache: skip malformed modules.yaml doc", "error", err)

			continue
		}

		switch doc.Document {
		case "modulemd":
			if doc.Data.Name == "" || doc.Data.Stream == "" {
				continue
			}

			key := doc.Data.Name + ":" + doc.Data.Stream
			streamRPMs[key] = append(streamRPMs[key], doc.Data.Artifacts.RPMs...)

		case "modulemd-defaults":
			// In modulemd-defaults docs, `stream` is the chosen default
			// stream for `module`.
			if doc.Data.Module == "" || doc.Data.Stream == "" {
				continue
			}
			// First default wins (repos shouldn't disagree, but be safe).
			if _, ok := idx.defaultStream[doc.Data.Module]; !ok {
				idx.defaultStream[doc.Data.Module] = doc.Data.Stream
			}
		}
	}

	buildBlockedNVRA(idx, streamRPMs)
}

// buildBlockedNVRA fills idx.blockedNVRA with NVRAs from NON-default streams
// of each module. Denylist (not allowlist) because some Rocky/RHEL 8 minor
// releases (e.g. 8.10) ship the default-stream modulemd document with an
// empty artifacts list while the default-stream `.module+` RPMs still live
// in primary.xml — an allowlist would over-filter and drop them entirely
// (e.g. perl-interpreter on Rocky 8.10).
func buildBlockedNVRA(idx *moduleIndex, streamRPMs map[string][]string) {
	for module, defaultStream := range idx.defaultStream {
		prefix := module + ":"
		defaultKey := prefix + defaultStream

		for key, rpms := range streamRPMs {
			if !strings.HasPrefix(key, prefix) || key == defaultKey {
				continue
			}

			for _, nvra := range rpms {
				idx.blockedNVRA[nvra] = true
			}
		}
	}
}

// parseModulesFile opens path (possibly .gz/.xz compressed) and merges its
// modulemd content into idx.
func parseModulesFile(path string, idx *moduleIndex) error {
	f, err := os.Open(path) //nolint:gosec
	if err != nil {
		return err
	}

	defer func() { _ = f.Close() }()

	var r io.Reader = f

	switch {
	case strings.HasSuffix(path, ".gz"):
		gz, err := gzip.NewReader(f)
		if err != nil {
			return err
		}

		defer func() { _ = gz.Close() }()

		r = gz

	case strings.HasSuffix(path, ".xz"):
		xzr, err := xz.NewReader(bufio.NewReader(f))
		if err != nil {
			return err
		}

		r = xzr
	}

	parseModulesYAML(r, idx)

	return nil
}

// isModularPackage reports whether a package's Release tag marks it as a
// module-stream build. Rocky/RHEL modular packages always carry
// `.module+elN.M.0+...` in the Release field.
func isModularPackage(release string) bool {
	return strings.Contains(release, ".module+")
}

// packageNVRA builds the NVRA string used as a key in modules.yaml
// artifacts: "name-epoch:version-release.arch". Epoch is always explicit
// and defaults to "0" when absent in primary.xml.
func packageNVRA(name, epoch, version, release, arch string) string {
	if epoch == "" {
		epoch = "0"
	}

	return name + "-" + epoch + ":" + version + "-" + release + "." + arch
}

// isModuleIndex reports whether name is a modules.yaml file from repodata.
func isModuleIndex(name string) bool {
	base := name
	for _, ext := range []string{".gz", ".xz", ".zst"} {
		base = strings.TrimSuffix(base, ext)
	}

	return strings.HasSuffix(base, "modules.yaml") || strings.HasSuffix(base, "-modules.yaml")
}

// collectModuleFiles walks the on-disk repo cache and returns the list of
// modules.yaml files to parse.
func collectModuleFiles() []string {
	repos := parseRepoFiles()

	var files []string

	for _, repo := range repos {
		if !repo.Enabled {
			continue
		}

		cacheDir := findRepoCacheDir(repo.ID)
		if cacheDir == "" {
			continue
		}

		repoCache := filepath.Join(cacheDir, "repodata")

		entries, err := os.ReadDir(repoCache)
		if err != nil {
			continue
		}

		for _, e := range entries {
			if e.IsDir() || !isModuleIndex(e.Name()) {
				continue
			}

			files = append(files, filepath.Join(repoCache, e.Name()))
		}
	}

	return files
}

// loadModuleIndex parses all available modules.yaml files into a fresh
// moduleIndex. Returns an empty (non-nil) index on non-RPM hosts or when
// no module metadata is present.
func loadModuleIndex() *moduleIndex {
	idx := newModuleIndex()

	files := collectModuleFiles()
	for _, f := range files {
		if err := parseModulesFile(f, idx); err != nil {
			logger.Warn("dnfcache: failed to parse modules.yaml",
				"file", f,
				"error", err)
		}
	}

	if len(idx.defaultStream) > 0 {
		logger.Info("dnfcache: module index loaded",
			"files", len(files),
			"defaults", len(idx.defaultStream),
			"blocked_nvra", len(idx.blockedNVRA))
	}

	return idx
}
