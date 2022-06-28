/*
Copyright © 2022 Samir Kuthiala <samir.kuthiala@gmail.com>

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in
all copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
THE SOFTWARE.
*/
package cmd

import (
	"archive/tar"
	"io"
	"os"
	"path/filepath"

	"github.com/google/go-containerregistry/pkg/crane"
	"github.com/google/uuid"
	"github.com/spf13/cobra"
)

// runCmd represents the run command
var runCmd = &cobra.Command{
	Use:   "run image-path",
	Short: "Run the container",
	Long:  `Run the container based on options passed in`,
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		imageRef := args[0]

		cacheDir, err := cmd.Flags().GetString("cache")
		if err != nil {
			return err
		}

		forceDownload, err := cmd.Flags().GetBool("force")
		if err != nil {
			return err
		}

		err = prepContainer(imageRef, cacheDir, forceDownload)
		if err != nil {
			return err
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(runCmd)

	runCmd.Flags().String("cache", "/tmp/rcon", "cache folder")
	runCmd.Flags().Bool("force", false, "force download the image ignoring cache")
}

func prepContainer(imageRef, cacheDir string, forceDownload bool) error {
	// download image manifest
	img, err := crane.Pull(imageRef)
	if err != nil {
		return err
	}

	imgHash, err := img.ConfigName()
	if err != nil {
		return err
	}

	imgId := imgHash.String()

	// download and export if not in cache
	exportDir := filepath.Join(cacheDir, imgId)
	os.MkdirAll(exportDir, 0644)
	tarFile := filepath.Join(exportDir, "fs.tar")
	if _, err := os.Stat(tarFile); os.IsNotExist(err) || forceDownload {
		f, err := os.Create(tarFile)
		if err != nil {
			return err
		}

		err = crane.Export(img, f)
		if err != nil {
			f.Close()
			return err
		}

		f.Close()
	}

	// extract filesystem
	rootFS := filepath.Join(exportDir, uuid.NewString())
	os.MkdirAll(rootFS, 0644)
	err = Untar(tarFile, rootFS)
	if err != nil {
		return err
	}

	// extract config

	return nil
}

func Untar(tarFile, dst string) error {
	f, err := os.Open(tarFile)
	if err != nil {
		return err
	}
	defer f.Close()

	tr := tar.NewReader(f)

	for {
		header, err := tr.Next()

		switch {

		// if no more files are found return
		case err == io.EOF:
			return nil

		// return any other error
		case err != nil:
			return err

		// if the header is nil, just skip it (not sure how this happens)
		case header == nil:
			continue
		}

		// the target location where the dir/file should be created
		target := filepath.Join(dst, header.Name)

		// the following switch could also be done using fi.Mode(), not sure if there
		// a benefit of using one vs. the other.
		// fi := header.FileInfo()

		// check the file type
		switch header.Typeflag {

		// if its a dir and it doesn't exist create it
		case tar.TypeDir:
			if _, err := os.Stat(target); err != nil {
				if err := os.MkdirAll(target, 0755); err != nil {
					return err
				}
			}

		// if it's a file create it
		case tar.TypeReg:
			f, err := os.OpenFile(target, os.O_CREATE|os.O_RDWR, os.FileMode(header.Mode))
			if err != nil {
				return err
			}

			// copy over contents
			if _, err := io.Copy(f, tr); err != nil {
				return err
			}

			// manually close here after each file operation; defering would cause each file close
			// to wait until all operations have completed.
			f.Close()
		}
	}
}
