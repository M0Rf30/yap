package options

// Options holds the resolved flags from a PKGBUILD options array.
type Options struct {
	DebugEnabled     bool
	DocsEnabled      bool
	EmptyDirsEnabled bool
	LibtoolEnabled   bool
	PurgeEnabled     bool
	StaticEnabled    bool
	StripEnabled     bool
	ZipManEnabled    bool
}

// Apply runs all enabled/disabled option handlers against packageDir in the
// correct order (strip first, then cleanup passes). STRIP/OBJCOPY are read
// from the process environment; use ApplyWithEnv for parallel-safe overrides.
func Apply(packageDir string, o Options) error {
	return ApplyWithEnv(packageDir, o, nil)
}

// ApplyWithEnv is the env-overlay variant of Apply. The env map is consulted
// before os.Getenv for STRIP/OBJCOPY during the strip pass, allowing parallel
// builds to scope cross-compilation toolchain selection without mutating the
// global process environment.
func ApplyWithEnv(packageDir string, o Options, env map[string]string) error {
	if o.StripEnabled {
		if err := StripWithEnv(packageDir, env); err != nil {
			return err
		}
	}

	if !o.DocsEnabled {
		if err := RemoveDocs(packageDir); err != nil {
			return err
		}
	}

	if !o.LibtoolEnabled {
		if err := RemoveLibtool(packageDir); err != nil {
			return err
		}
	}

	if !o.StaticEnabled {
		if err := RemoveStatic(packageDir); err != nil {
			return err
		}
	}

	if o.PurgeEnabled {
		if err := Purge(packageDir); err != nil {
			return err
		}
	}

	if o.ZipManEnabled {
		if err := ZipMan(packageDir); err != nil {
			return err
		}
	}

	// Run empty-dirs last so previous passes can create new empty dirs.
	if !o.EmptyDirsEnabled {
		if err := RemoveEmptyDirs(packageDir); err != nil {
			return err
		}
	}

	// NOTE: DebugEnabled is intentionally not handled here. Debug symbol separation
	// is handled separately via the --debug-dir flag and the Strip() function in
	// strip.go.

	return nil
}
