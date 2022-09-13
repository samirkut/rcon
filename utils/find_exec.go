package utils

import (
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
)

func findExecutable(file string) error {
	d, err := os.Stat(file)
	if err != nil {
		return err
	}

	m := d.Mode()
	if m.IsDir() {
		return syscall.EISDIR
	}

	fi, err := os.Stat(file)
	if err != nil {
		return err
	}

	if fi.Mode()&0111 != 0 {
		return nil
	}

	return fs.ErrPermission
}

// adapted from exec.LookupPath (from go source code)
func FindExecInPath(file string, path string) (string, error) {
	if strings.Contains(file, "/") {
		err := findExecutable(file)
		if err == nil {
			return file, nil
		}
		return "", err
	}

	if path == "" {
		path = os.Getenv("PATH")
	}

	for _, dir := range filepath.SplitList(path) {
		if dir == "" {
			// Unix shell semantics: path element "" means "."
			dir = "."
		}
		path := filepath.Join(dir, file)
		if err := findExecutable(path); err == nil {
			return path, nil
		}
	}

	return "", exec.ErrNotFound
}

func Findenv(env []string, key string) string {
	value := ""

	// the last entry is used in case of duplicate keys
	for _, kv := range env {
		arr := strings.Split(kv, "=")
		if len(arr) != 2 {
			continue
		}
		if arr[0] == key {
			value = arr[1]
		}
	}

	return value
}
