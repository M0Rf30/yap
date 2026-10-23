// SPDX-FileCopyrightText: 2021-2026 Gianluca Boiano
// SPDX-License-Identifier: GPL-3.0-only

package publish

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/M0Rf30/yap/v2/pkg/errors"
)

// ProjectFileName is the yap project file read for the "publish" section.
const ProjectFileName = "yap.json"

// LoadProjectFile reads the "publish" object and the "output" directory from a
// yap.json. path is the file itself or the directory containing it. The
// returned output is resolved against the file's directory ("" when the file
// has none).
func LoadProjectFile(path string) (cfg Config, output string, err error) {
	file := path

	if info, statErr := os.Stat(path); statErr == nil && info.IsDir() {
		file = filepath.Join(path, ProjectFileName)
	}

	data, err := os.ReadFile(file) //nolint:gosec // user-selected project file
	if err != nil {
		return Config{}, "", errors.Wrap(err, errors.ErrTypeFileSystem, "cannot read project file").
			WithOperation("LoadProjectFile").
			WithContext("path", file)
	}

	var doc struct {
		Output  string  `json:"output"`
		Publish *Config `json:"publish"`
	}

	if err := json.Unmarshal(data, &doc); err != nil {
		return Config{}, "", errors.Wrap(err, errors.ErrTypeParser, "invalid project file").
			WithOperation("LoadProjectFile").
			WithContext("path", file)
	}

	if doc.Publish != nil {
		cfg = *doc.Publish
	}

	output = doc.Output
	if output != "" && !filepath.IsAbs(output) {
		output = filepath.Join(filepath.Dir(file), output)
	}

	return cfg, output, nil
}
