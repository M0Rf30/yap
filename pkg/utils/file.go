	"github.com/pkg/errors"
		Logger.Error("failed to chmod", Logger.Args("path", path))

		Logger.Error("failed to create path", Logger.Args("path", path))
		Logger.Error("failed to write to file", Logger.Args("path", path))
			return errors.Errorf("failed to create directory %s", path)
		return errors.Errorf("failed to access directory %s", path)
			Logger.Fatal("failed to get dir size",
				Logger.Args("path", path))
		Logger.Error("failed to make directory",
			Logger.Args("path", path))
		Logger.Error("failed to open file",
			Logger.Args("path", path))
		Logger.Error("failed to remove",
			Logger.Args("path", path))