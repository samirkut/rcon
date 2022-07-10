package utils

import (
	"os"
	"path/filepath"
	"strings"
)

func ExpandPath(path string) (string, error) {
	// expand ~/ to current dir
	if strings.HasPrefix(path, "~/") {
		dirname, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		path = filepath.Join(dirname, path[2:])
	}

	return path, nil
}

func PathExists(path string) bool {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return false
	} else if err != nil {
		return false //?
	}
	return true
}

func FileSize(file string) (int64, error) {
	info, err := os.Stat(file)
	if err != nil {
		return 0, err
	}

	return info.Size(), nil
}

func EnsureDir(path string) (string, error) {
	path, err := ExpandPath(path)
	if err != nil {
		return "", err
	}

	err = os.MkdirAll(path, 0755)
	if err != nil {
		return "", err
	}

	return path, nil
}
