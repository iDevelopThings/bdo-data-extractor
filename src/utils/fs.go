package utils

import "os"

// EnsureDirCreated creates the directory at path if it does not exist, including any
// necessary parent directories. Returns an error if the directory cannot be created.
func EnsureDirCreated(path string) error {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		err := os.MkdirAll(path, 0755)
		if err != nil {
			return err
		}
	}
	return nil
}
