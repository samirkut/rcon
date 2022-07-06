package container

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"syscall"
)

func MountProc(newroot string) error {
	source := "proc"
	target := filepath.Join(newroot, "/proc")
	fstype := "proc"
	flags := 0
	data := ""

	if err := os.MkdirAll(target, 0755); err != nil {
		return err
	}

	if err := syscall.Mount(source, target, fstype, uintptr(flags), data); err != nil {
		return err
	}

	return nil
}

func MountBind(source, target string) error {
	// create target directory
	if source != target {
		if err := os.MkdirAll(target, 0700); err != nil {
			return err
		}
	}

	return syscall.Mount(source, target, "", syscall.MS_BIND|syscall.MS_REC, "")
}

func MountTmpfs(path string, size int64) error {
	if size < 0 {
		return errors.New("MountTmpfs: size < 0")
	}

	var flags uintptr
	flags = syscall.MS_NOATIME | syscall.MS_SILENT
	flags |= syscall.MS_NODEV | syscall.MS_NOEXEC | syscall.MS_NOSUID

	options := ""
	if size >= 0 {
		options = "size=" + strconv.FormatInt(size, 10)
	}

	return syscall.Mount("tmpfs", path, "tmpfs", flags, options)
}
