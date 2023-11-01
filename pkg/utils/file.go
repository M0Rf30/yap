	"log"
// Chmod changes the file mode of the specified path.
//
// It takes a string parameter `path` which represents the path of the file
// whose mode needs to be changed. It also takes an `os.FileMode` parameter
// `perm` which represents the new file mode to be set.
//
// The function returns an error if there was an issue changing the file
// mode. If the file mode was successfully changed, it returns nil.
	return nil
// Create creates a new file at the specified path.
//
// It takes a string parameter `path` which represents the path of the file to be created.
// The function returns a pointer to an `os.File` and an error.
// CreateWrite writes the given data to the file specified by the path.
//
// It takes two parameters:
// - path: a string representing the path of the file.
// - data: a string representing the data to be written to the file.
//
// It returns an error if there was a problem creating or writing to the file.
	return nil
// Exists checks if a file or directory exists at the given path.
//
// path: the path to the file or directory.
// bool: returns true if the file or directory exists, false otherwise.
func Exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil || !os.IsNotExist(err)
// ExistsMakeDir checks if a directory exists at the given path and creates it if it doesn't.
//
// path: the path to the directory.
// error: returns an error if the directory cannot be created or accessed.
	if _, err := os.Stat(path); os.IsNotExist(err) {
		if err := os.MkdirAll(path, os.ModePerm); err != nil {
			return fmt.Errorf("failed to create directory '%s': %w", path, err)
	} else if err != nil {
		return fmt.Errorf("failed to access directory '%s': %w", path, err)
	return nil
// Filename returns the filename from a given path.
//
// It takes a string parameter `path` which represents the path of the file.
// It returns a string which is the filename extracted from the path.
// GetDirSize calculates the size of a directory in kilobytes.
//
// It takes a path as a parameter and returns the size of the directory in kilobytes and an error if any.
			log.Fatalf("%s❌ :: %sfailed to get dir size '%s'%s\n",
// MkdirAll creates a directory and all its parent directories.
//
// It takes a string parameter `path` which represents the path of the directory to be created.
// The function returns an error if any error occurs during the directory creation process.
	return nil
// Open opens a file at the specified path and returns a pointer to the file and an error.
//
// The path parameter is a string representing the file path to be opened.
// The function returns a pointer to an os.File and an error.
// Remove deletes a file or directory at the specified path.
//
// path: the path of the file or directory to be removed.
// Returns an error if the removal fails.
	return nil
// RemoveAll removes a file or directory and any children it contains.
//
// path: the path of the file or directory to be removed.
// error: an error if the removal fails.
	return nil