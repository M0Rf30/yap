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
// correct order (strip first, then cleanup passes).
func Apply(packageDir string, o Options) error {
	if o.StripEnabled {
		if err := Strip(packageDir); err != nil {
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

	return nil
}
