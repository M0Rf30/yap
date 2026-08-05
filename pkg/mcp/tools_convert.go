package mcp

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/M0Rf30/yap/v2/pkg/errors"
	"github.com/M0Rf30/yap/v2/pkg/nfpm"
	"github.com/M0Rf30/yap/v2/pkg/parser"
)

// convertPackagers is the set of nfpm packager names convert_spec accepts
// for the `packager` arg — the subset of nfpm.Packagers that maps onto a
// yap builder (pkg/nfpm has no `ipk` builder).
//
//nolint:gochecknoglobals // read-only lookup set, mirrors nfpm.Packagers
var convertPackagers = map[string]bool{
	nfpm.PackagerDeb:       true,
	nfpm.PackagerRPM:       true,
	nfpm.PackagerAPK:       true,
	nfpm.PackagerArchLinux: true,
}

// ----- convert_spec ----------------------------------------------------

type convertSpecArgs struct {
	SpecPath     string `json:"specPath" jsonschema:"nfpm.yaml/.nfpm.yml or PKGBUILD path, or its containing dir"`
	To           string `json:"to,omitempty" jsonschema:"target dialect: pkgbuild|nfpm; default: opposite of the input"`
	Packager     string `json:"packager,omitempty" jsonschema:"deb|rpm|apk|archlinux; resolve overrides for one packager"`
	OutputPath   string `json:"outputPath,omitempty" jsonschema:"write the rendered spec here instead of returning inline"`
	ContentsFrom string `json:"contentsFrom,omitempty" jsonschema:"pkgbuild->nfpm only: staged dir to walk into contents:"`
}

type convertSpecResult struct {
	Detected   string   `json:"detected" jsonschema:"detected input dialect: pkgbuild|nfpm"`
	To         string   `json:"to" jsonschema:"target dialect actually rendered"`
	OutputPath string   `json:"outputPath,omitempty" jsonschema:"file written; empty for the text-return path"`
	Bytes      int      `json:"bytes,omitempty" jsonschema:"byte count of the rendered spec"`
	Rendered   string   `json:"rendered,omitempty" jsonschema:"rendered spec text; empty when outputPath was set"`
	Dropped    []string `json:"dropped,omitempty" jsonschema:"messages for source features with no equivalent in target"`
}

func registerConvertSpec(srv *mcpsdk.Server) {
	mcpsdk.AddTool(srv, &mcpsdk.Tool{
		Name: toolNameConvertSpec,
		Description: "Convert between an nfpm.yaml spec and a YAP PKGBUILD, in either direction. " +
			"Detects the input dialect automatically; writes to outputPath when given, otherwise " +
			"returns the rendered spec text. Dropped-feature messages (source constructs with no " +
			"equivalent in the target dialect) are reported alongside the rendered output.",
		Annotations: &mcpsdk.ToolAnnotations{DestructiveHint: hintTrue, IdempotentHint: true},
	}, func(_ context.Context, _ *mcpsdk.CallToolRequest, args convertSpecArgs,
	) (*mcpsdk.CallToolResult, convertSpecResult, error) {
		res, err := runConvertSpec(&args)
		if err != nil {
			return nil, convertSpecResult{}, err
		}

		text := convertSpecTextResult(&res)

		return text, res, nil
	})
}

// convertSpecTextResult renders the human-facing tool content: the written
// confirmation or the rendered text, followed by a "Dropped features"
// heading listing any lossy conversions. The structured convertSpecResult
// (returned alongside) carries the same data for programmatic clients.
func convertSpecTextResult(res *convertSpecResult) *mcpsdk.CallToolResult {
	var b strings.Builder

	if res.OutputPath != "" {
		fmt.Fprintf(&b, "Wrote %d bytes to %s (%s -> %s).\n", res.Bytes, res.OutputPath, res.Detected, res.To)
	} else {
		b.WriteString(res.Rendered)

		if !strings.HasSuffix(res.Rendered, "\n") {
			b.WriteString("\n")
		}
	}

	if len(res.Dropped) > 0 {
		b.WriteString("\n## Dropped features\n")

		for _, msg := range res.Dropped {
			fmt.Fprintf(&b, "- %s\n", msg)
		}
	}

	return &mcpsdk.CallToolResult{Content: []mcpsdk.Content{&mcpsdk.TextContent{Text: b.String()}}}
}

// runConvertSpec is the thin, transport-agnostic body of convert_spec: it
// resolves the input dialect, dispatches to pkg/nfpm's converters/renderers,
// and either writes or returns the result. No mapping/build logic lives
// here — it all belongs to pkg/nfpm and pkg/parser.
func runConvertSpec(args *convertSpecArgs) (convertSpecResult, error) {
	if args.Packager != "" && !convertPackagers[args.Packager] {
		return convertSpecResult{}, errors.New(errors.ErrTypeValidation,
			"invalid packager: expected deb, rpm, apk, or archlinux").WithContext("packager", args.Packager)
	}

	abs, err := filepath.Abs(args.SpecPath)
	if err != nil {
		return convertSpecResult{}, errors.Wrap(err, errors.ErrTypeFileSystem,
			"resolve specPath").WithContext("specPath", args.SpecPath)
	}

	kind, specFile, dir, err := detectConvertInput(abs)
	if err != nil {
		return convertSpecResult{}, err
	}

	to, err := resolveConvertTarget(kind, args.To)
	if err != nil {
		return convertSpecResult{}, err
	}

	var (
		rendered []byte
		dropped  []string
	)

	switch kind {
	case parser.SpecNFPM:
		outputDir := ""
		if args.OutputPath != "" {
			outputDir = filepath.Dir(args.OutputPath)
		}

		rendered, dropped, err = convertNfpmToPKGBUILD(specFile, args.Packager, outputDir)
	case parser.SpecPKGBUILD:
		rendered, dropped, err = convertPKGBUILDToNfpm(dir, args)
	}

	if err != nil {
		return convertSpecResult{}, err
	}

	res := convertSpecResult{Detected: string(kind), To: to, Dropped: dropped, Bytes: len(rendered)}

	if args.OutputPath == "" {
		res.Rendered = string(rendered)
		return res, nil
	}

	if err := writeConvertOutput(args.OutputPath, rendered); err != nil {
		return convertSpecResult{}, err
	}

	res.OutputPath = args.OutputPath

	return res, nil
}

// detectConvertInput resolves specPath (a spec file or its containing
// directory) to the recognised dialect, the absolute spec file path, and
// the directory containing it.
func detectConvertInput(path string) (kind parser.SpecKind, specFile, dir string, err error) {
	info, statErr := os.Stat(path)
	if statErr != nil {
		return "", "", "", errors.Wrap(statErr, errors.ErrTypeFileSystem,
			"stat specPath").WithContext("path", path)
	}

	if info.IsDir() {
		k, file, found := parser.DetectSpec(path)
		if !found {
			return "", "", "", errors.New(errors.ErrTypeFileSystem,
				"no PKGBUILD or nfpm spec found in directory").WithContext("dir", path)
		}

		return k, file, path, nil
	}

	switch {
	case filepath.Base(path) == pkgbuildFileName:
		return parser.SpecPKGBUILD, path, filepath.Dir(path), nil
	case nfpm.IsSpecFile(path):
		return parser.SpecNFPM, path, filepath.Dir(path), nil
	default:
		return "", "", "", errors.New(errors.ErrTypeFileSystem,
			"unrecognised spec file: expected PKGBUILD or nfpm.yaml").WithContext("path", path)
	}
}

// resolveConvertTarget validates the requested `to` dialect (defaulting to
// the opposite of kind) and rejects a same-dialect request — there is no
// renderer that round-trips a dialect onto itself.
func resolveConvertTarget(kind parser.SpecKind, to string) (string, error) {
	switch to {
	case "":
		if kind == parser.SpecNFPM {
			return "pkgbuild", nil
		}

		return "nfpm", nil
	case "pkgbuild", "nfpm":
		if to == string(kind) {
			return "", errors.New(errors.ErrTypeValidation,
				"specPath is already that dialect; nothing to convert").WithContext("dialect", to)
		}

		return to, nil
	default:
		return "", errors.New(errors.ErrTypeValidation,
			"invalid to: expected pkgbuild or nfpm").WithContext("to", to)
	}
}

// convertNfpmToPKGBUILD loads an nfpm.yaml at specFile and renders it as
// YAP specfile text. With packager set, overrides are collapsed onto a
// single packager's Info (no yaml override plumbing/suffixed directives);
// without it, the full multi-packager suffixed spec is emitted. outputDir
// controls where a changelog=-referenced sidecar file is written; empty
// reports that instead via the returned dropped messages.
func convertNfpmToPKGBUILD(
	specFile, packager, outputDir string,
) (rendered []byte, dropped []string, err error) {
	cfg, err := nfpm.Load(specFile)
	if err != nil {
		return nil, nil, errors.Wrap(err, errors.ErrTypeParser, "load nfpm spec").WithContext("path", specFile)
	}

	renderCfg := cfg

	if packager != "" {
		info, ferr := cfg.ForPackager(packager)
		if ferr != nil {
			return nil, nil, errors.Wrap(ferr, errors.ErrTypeParser,
				"resolve nfpm overrides").WithContext("packager", packager)
		}

		renderCfg = &nfpm.Config{Info: *info}
	}

	rendered, dropped, err = renderCfg.RenderPKGBUILD(&nfpm.RenderOptions{OutputDir: outputDir})
	if err != nil {
		return nil, nil, errors.Wrap(err, errors.ErrTypeParser, "render PKGBUILD").WithContext("packager", packager)
	}

	return rendered, dropped, nil
}

// convertPKGBUILDToNfpm parses the PKGBUILD in dir and renders it as
// nfpm.yaml text. outputDir controls where FromPKGBUILD writes sibling
// scriptlet files (<name>.<hook>.sh) referenced by scripts.* in the YAML;
// it is the outputPath's directory when set, otherwise empty so
// FromPKGBUILD reports a dropped-feature message instead of writing to a
// throwaway location.
func convertPKGBUILDToNfpm(dir string, args *convertSpecArgs) (rendered []byte, dropped []string, err error) {
	if args.ContentsFrom != "" {
		dropped = append(dropped, "contentsFrom is ignored when converting nfpm->pkgbuild")
	}

	p, err := parser.ParseFile("", "", dir, dir, "")
	if err != nil {
		return nil, nil, errors.Wrap(err, errors.ErrTypeParser, "parse PKGBUILD").WithContext("dir", dir)
	}

	outputDir := ""
	if args.OutputPath != "" {
		outputDir = filepath.Dir(args.OutputPath)
	}

	cfg, fromDropped, err := nfpm.FromPKGBUILD(p, nfpm.FromOptions{
		Packager:     args.Packager,
		ContentsFrom: args.ContentsFrom,
		OutputDir:    outputDir,
	})
	if err != nil {
		return nil, nil, errors.Wrap(err, errors.ErrTypeParser, "convert PKGBUILD to nfpm").WithContext("dir", dir)
	}

	dropped = append(dropped, fromDropped...)

	rendered, err = cfg.RenderYAML()
	if err != nil {
		return nil, dropped, errors.Wrap(err, errors.ErrTypeParser, "render nfpm.yaml")
	}

	return rendered, dropped, nil
}

// writeConvertOutput writes rendered to outputPath, creating its parent
// directory when missing.
func writeConvertOutput(outputPath string, rendered []byte) error {
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o750); err != nil {
		return errors.Wrap(err, errors.ErrTypeFileSystem, "create output directory").WithContext("path", outputPath)
	}

	if err := os.WriteFile(outputPath, rendered, 0o644); err != nil { //nolint:gosec // spec text, not a secret
		return errors.Wrap(err, errors.ErrTypeFileSystem, "write output").WithContext("path", outputPath)
	}

	return nil
}
