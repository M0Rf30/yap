package command

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/M0Rf30/yap/v2/pkg/errors"
	"github.com/M0Rf30/yap/v2/pkg/files"
	"github.com/M0Rf30/yap/v2/pkg/i18n"
	"github.com/M0Rf30/yap/v2/pkg/logger"
	"github.com/M0Rf30/yap/v2/pkg/nfpm"
	"github.com/M0Rf30/yap/v2/pkg/parser"
)

// Specfile dialect names accepted by --to and returned by dialect detection.
const (
	dialectPKGBUILD = "pkgbuild"
	dialectNFPM     = "nfpm"
)

var (
	convertOutput       string
	convertTo           string
	convertPackager     string
	convertContentsFrom string
	convertForce        bool
)

// convertCmd converts a specfile between YAP's extended-PKGBUILD dialect and
// an nfpm.yaml document, in either direction.
var convertCmd = &cobra.Command{
	Use:     commandConvert + " <spec>",
	GroupID: commandUtility,
	Short:   "", // Set by InitializeLocalizedDescriptions
	Long:    "", // Set by InitializeLocalizedDescriptions
	Example: "", // Set by InitializeLocalizedDescriptions
	Args:    cobra.ExactArgs(1),
	RunE:    runConvertCommand,
}

// InitializeConvertDescriptions sets the localized descriptions for convert.
func InitializeConvertDescriptions() {
	initCommandDescriptions(convertCmd, commandConvert, map[string]string{
		"output":        "flags.convert.output",
		"to":            "flags.convert.to",
		"packager":      "flags.convert.packager",
		"contents-from": "flags.convert.contents_from",
		"force":         "flags.convert.force",
	})
}

//nolint:gochecknoinits // Required for cobra command registration
func init() {
	rootCmd.AddCommand(convertCmd)

	convertCmd.Flags().StringVarP(&convertOutput, "output", "o", "", "")
	convertCmd.Flags().StringVar(&convertTo, "to", "", "")
	convertCmd.Flags().StringVar(&convertPackager, "packager", "", "")
	convertCmd.Flags().StringVar(&convertContentsFrom, "contents-from", "", "")
	convertCmd.Flags().BoolVarP(&convertForce, "force", "f", false, "")
}

// runConvertCommand implements `yap convert <spec>`.
func runConvertCommand(cmd *cobra.Command, args []string) error {
	specPath, err := resolveSpecInput(args[0])
	if err != nil {
		return err
	}

	fromKind, err := detectDialect(specPath)
	if err != nil {
		return err
	}

	toKind, err := resolveTargetDialect(fromKind)
	if err != nil {
		return err
	}

	output, err := resolveOutputTarget(toKind, specPath)
	if err != nil {
		return err
	}

	rendered, messages, err := convertSpec(fromKind, specPath, output)
	if err != nil {
		return err
	}

	for _, msg := range messages {
		logger.Warn(i18n.T("logger.convert.warn.unsupported_field"), "detail", msg)
	}

	if err := writeConvertOutput(cmd, output, rendered); err != nil {
		return err
	}

	logger.Info(i18n.T("logger.convert.written"), "path", output, "from", fromKind, "to", toKind)

	return nil
}

// resolveTargetDialect determines the conversion target: --to when set (validated
// against the two known dialects), else the dialect opposite fromKind. Converting a
// dialect to itself is rejected.
func resolveTargetDialect(fromKind string) (string, error) {
	toKind := convertTo
	if toKind == "" {
		toKind = oppositeDialect(fromKind)
	} else if toKind != dialectPKGBUILD && toKind != dialectNFPM {
		return "", errors.New(errors.ErrTypeValidation,
			fmt.Sprintf(i18n.T("errors.convert.invalid_target"), toKind)).
			WithOperation("resolveTargetDialect").
			WithContext("to", toKind)
	}

	if fromKind == toKind {
		return "", errors.New(errors.ErrTypeValidation,
			fmt.Sprintf(i18n.T("errors.convert.same_dialect"), toKind)).
			WithOperation("resolveTargetDialect").
			WithContext("dialect", toKind)
	}

	return toKind, nil
}

// resolveOutputTarget determines the output path for a toKind conversion of specPath:
// --output when set, else the conventional filename beside specPath. Refuses to
// clobber an existing file unless --force is set; "-" (stdout) is never checked.
func resolveOutputTarget(toKind, specPath string) (string, error) {
	output := convertOutput
	if output == "" {
		output = defaultOutputPath(toKind, filepath.Dir(specPath))
	}

	if output != "-" && files.Exists(output) && !convertForce {
		return "", errors.New(errors.ErrTypeValidation,
			fmt.Sprintf(i18n.T("errors.convert.output_exists"), output)).
			WithOperation("resolveOutputTarget").
			WithContext("path", output)
	}

	return output, nil
}

// convertSpec dispatches to the direction-specific converter for specPath, given the
// already-resolved output path (used to derive OutputDir for pkgbuild->nfpm sidecar
// scripts and nfpm->pkgbuild changelog sidecars alike).
func convertSpec(fromKind, specPath, output string) (rendered []byte, dropped []string, err error) {
	outputDir := ""
	if output != "-" {
		outputDir = filepath.Dir(output)
	}

	if fromKind == dialectNFPM {
		return convertNfpmToPKGBUILD(specPath, outputDir)
	}

	return convertPKGBUILDToNfpm(filepath.Dir(specPath), outputDir)
}

// oppositeDialect returns the dialect not equal to kind.
func oppositeDialect(kind string) string {
	if kind == dialectNFPM {
		return dialectPKGBUILD
	}

	return dialectNFPM
}

// defaultOutputPath returns the conventional output filename for toKind,
// placed beside the input spec (in dir).
func defaultOutputPath(toKind, dir string) string {
	if toKind == dialectPKGBUILD {
		return filepath.Join(dir, "PKGBUILD")
	}

	return filepath.Join(dir, "nfpm.yaml")
}

// resolveSpecInput accepts either a specfile path or a directory containing
// one, and returns the resolved specfile path. A PKGBUILD in the directory
// takes precedence over an nfpm spec, matching parser.DetectSpec.
func resolveSpecInput(input string) (string, error) {
	info, err := os.Stat(input)
	if err != nil {
		return "", errors.Wrap(err, errors.ErrTypeFileSystem, i18n.T("errors.convert.path_not_found")).
			WithOperation("resolveSpecInput").
			WithContext("path", input)
	}

	if !info.IsDir() {
		return input, nil
	}

	pkgbuildCandidate := filepath.Join(input, "PKGBUILD")
	if files.Exists(pkgbuildCandidate) {
		return pkgbuildCandidate, nil
	}

	if specFile, ok := nfpm.FindSpec(input); ok {
		return specFile, nil
	}

	return "", errors.New(errors.ErrTypeValidation, i18n.T("errors.convert.no_spec_in_dir")).
		WithOperation("resolveSpecInput").
		WithContext("dir", input)
}

// detectDialect identifies which specfile dialect path is, by filename.
func detectDialect(path string) (string, error) {
	if nfpm.IsSpecFile(path) {
		return dialectNFPM, nil
	}

	if filepath.Base(path) == "PKGBUILD" {
		return dialectPKGBUILD, nil
	}

	return "", errors.New(errors.ErrTypeValidation,
		fmt.Sprintf(i18n.T("errors.convert.unrecognized_spec"), path)).
		WithOperation("detectDialect").
		WithContext("path", path)
}

// convertNfpmToPKGBUILD loads the nfpm spec at specPath and renders it as
// YAP specfile text. When --packager is set the config is first collapsed
// to that packager's overrides so the render carries no suffixed
// directives. outputDir, when non-empty, is where a changelog=-referenced
// sidecar file is written; empty reports that via the returned messages.
func convertNfpmToPKGBUILD(specPath, outputDir string) (rendered []byte, messages []string, err error) {
	cfg, err := nfpm.Load(specPath)
	if err != nil {
		return nil, nil, errors.Wrap(err, errors.ErrTypeParser, i18n.T("errors.convert.load_failed")).
			WithOperation("convertNfpmToPKGBUILD").
			WithContext("path", specPath)
	}

	renderCfg := cfg

	if convertPackager != "" {
		info, ferr := cfg.ForPackager(convertPackager)
		if ferr != nil {
			return nil, nil, errors.Wrap(ferr, errors.ErrTypeValidation,
				i18n.T("errors.convert.packager_failed")).
				WithOperation("convertNfpmToPKGBUILD").
				WithContext("packager", convertPackager)
		}

		renderCfg = &nfpm.Config{Info: *info}
	}

	rendered, messages, err = renderCfg.RenderPKGBUILD(&nfpm.RenderOptions{OutputDir: outputDir})
	if err != nil {
		return nil, nil, errors.Wrap(err, errors.ErrTypeInternal, i18n.T("errors.convert.render_failed")).
			WithOperation("convertNfpmToPKGBUILD").
			WithContext("to", dialectPKGBUILD)
	}

	return rendered, messages, nil
}

// convertPKGBUILDToNfpm parses the PKGBUILD in specDir and renders it as an
// nfpm.yaml document. outputDir, when non-empty, is where sibling scriptlet
// files get written by nfpm.FromPKGBUILD.
func convertPKGBUILDToNfpm(specDir, outputDir string) (rendered []byte, dropped []string, err error) {
	parsed, err := parser.ParseFile("", "", specDir, specDir, "")
	if err != nil {
		return nil, nil, errors.Wrap(err, errors.ErrTypeParser, i18n.T("errors.convert.parse_failed")).
			WithOperation("convertPKGBUILDToNfpm").
			WithContext("path", specDir)
	}

	cfg, messages, err := nfpm.FromPKGBUILD(parsed, nfpm.FromOptions{
		Packager:     convertPackager,
		ContentsFrom: convertContentsFrom,
		OutputDir:    outputDir,
	})
	if err != nil {
		return nil, nil, errors.Wrap(err, errors.ErrTypeInternal,
			i18n.T("errors.convert.from_pkgbuild_failed")).
			WithOperation("convertPKGBUILDToNfpm").
			WithContext("path", specDir)
	}

	rendered, err = cfg.RenderYAML()
	if err != nil {
		return nil, nil, errors.Wrap(err, errors.ErrTypeInternal, i18n.T("errors.convert.render_failed")).
			WithOperation("convertPKGBUILDToNfpm").
			WithContext("to", dialectNFPM)
	}

	return rendered, messages, nil
}

// writeConvertOutput writes content to output, or to cmd's stdout when
// output is "-", creating any missing parent directory first.
func writeConvertOutput(cmd *cobra.Command, output string, content []byte) error {
	if output == "-" {
		if _, err := cmd.OutOrStdout().Write(content); err != nil {
			return errors.Wrap(err, errors.ErrTypeFileSystem, i18n.T("errors.convert.write_failed")).
				WithOperation("writeConvertOutput").
				WithContext("path", output)
		}

		return nil
	}

	if err := files.ExistsMakeDir(filepath.Dir(output)); err != nil {
		return errors.Wrap(err, errors.ErrTypeFileSystem, i18n.T("errors.convert.write_failed")).
			WithOperation("writeConvertOutput").
			WithContext("path", output)
	}

	//nolint:gosec // spec output is not sensitive
	if err := os.WriteFile(output, content, 0o644); err != nil {
		return errors.Wrap(err, errors.ErrTypeFileSystem, i18n.T("errors.convert.write_failed")).
			WithOperation("writeConvertOutput").
			WithContext("path", output)
	}

	return nil
}
