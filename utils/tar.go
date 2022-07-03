package utils

import (
	"archive/tar"
	"io"
	"io/fs"
	"os"
	"path/filepath"
)

func Untar(tarball, target string) error {
	reader, err := os.Open(tarball)
	if err != nil {
		return err
	}
	defer reader.Close()
	tarReader := tar.NewReader(reader)

	delayedPerms := make(map[string]fs.FileMode)

	for {
		header, err := tarReader.Next()
		// if no more files are found return
		if err == io.EOF {
			break
		}
		// return any other error
		if err != nil {
			return err
		}
		// if the header is nil, just skip it (not sure how this happens)
		if header == nil {
			continue
		}

		path := filepath.Join(target, header.Name)
		info := header.FileInfo()

		switch header.Typeflag {
		case tar.TypeDir:
			dirMode := info.Mode() & 0777 // remove other bits
			// delay application if mode is not writeable or acessible
			if dirMode&0222 == 0 || dirMode&0111 == 0 {
				delayedPerms[path] = dirMode
				dirMode = 0755
			}
			if err = os.MkdirAll(path, dirMode); err != nil {
				return err
			}
			break
		case tar.TypeReg:
			file, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, info.Mode())
			if err != nil {
				return err
			}
			defer file.Close()
			_, err = io.Copy(file, tarReader)
			if err != nil {
				return err
			}
			break
		case tar.TypeSymlink:
			linkTarget := header.Linkname
			err = os.Symlink(linkTarget, path)
			if err != nil {
				return err
			}
			break
		}
	}

	// apply delayed perms
	for path, dirMode := range delayedPerms {
		err := os.Chmod(path, dirMode)
		if err != nil {
			return err
		}
	}

	return nil
}
