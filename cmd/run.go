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
	"encoding/base64"
	"encoding/json"
	"errors"
	"io/fs"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"rcon/utils"
	"strconv"
	"strings"
	"syscall"

	"github.com/google/go-containerregistry/pkg/crane"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/uuid"
	"github.com/spf13/cobra"
	"golang.org/x/sys/unix"
)

type TmpfsMount struct {
	Path string
	Size int64
}

type BindMount struct {
	Source string
	Target string
}

// runCmd represents the run command
var runCmd = &cobra.Command{
	Use:   "run image-path [command]",
	Short: "Run the container",
	Long:  `Run the container based on options passed in`,
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if os.Args[0] != "ns" {
			//reexec with namespace attrs
			args := []string{"ns"}
			args = append(args, os.Args[1:]...)
			cmd := reexecCmd(args...)
			return cmd.Run()
		}

		imageRef := args[0]

		runDir, err := cmd.Flags().GetString("run-dir")
		if err != nil {
			return err
		}

		runDir, err = utils.ExpandPath(runDir)
		if err != nil {
			return err
		}

		err = os.MkdirAll(runDir, 0755)
		if err != nil {
			return err
		}

		cacheDir, err := cmd.Flags().GetString("cache-dir")
		if err != nil {
			return err
		}

		cacheDir, err = utils.ExpandPath(cacheDir)
		if err != nil {
			return err
		}

		err = os.MkdirAll(cacheDir, 0755)
		if err != nil {
			return err
		}

		skipCache, err := cmd.Flags().GetBool("skip-cache")
		if err != nil {
			return err
		}

		mounts, err := cmd.Flags().GetStringArray("mount")
		if err != nil {
			return err
		}

		bindMounts := []BindMount{}
		tmpfsMounts := []TmpfsMount{}

		for _, mt := range mounts {
			arr := strings.Split(mt, ":")
			if len(arr) == 2 {
				//bind mount
				bindMounts = append(bindMounts, BindMount{
					Source: arr[0],
					Target: arr[1],
				})
			} else if len(arr) == 3 {
				//tmpfs mounts
				size, err := strconv.Atoi(arr[2])
				if err != nil {
					return err
				}
				tmpfsMounts = append(tmpfsMounts, TmpfsMount{
					Path: arr[0],
					Size: int64(size),
				})
			} else {
				return errors.New("mounts not defined correctly")
			}
		}

		err = fetchContainer(imageRef, cacheDir, skipCache)
		if err != nil {
			return err
		}

		rootFS, cfg, err := prepContainer(imageRef, cacheDir, runDir)
		if err != nil {
			return err
		}

		// clean up rootFS on exit
		defer func() {
			log.Println("Removing", rootFS)
			_ = os.RemoveAll(rootFS)
		}()

		return nsInitialisation(rootFS, cfg, bindMounts, tmpfsMounts, args[1:])
	},
}

func init() {
	rootCmd.AddCommand(runCmd)

	runCmd.Flags().String("run-dir", "/tmp/rcon/run", "cache folder for images")
	runCmd.Flags().String("cache-dir", "~/.rcon", "cache folder for images")
	runCmd.Flags().Bool("skip-cache", false, "use image in cache if possible")
	runCmd.Flags().StringArray("mount", nil, "mounts to pass in specified as host_path:container_path for bind mounts, or just container_path:tmpfs:size_bytes for tmpfs")
}

func fetchContainer(imageRef, cacheDir string, skipCache bool) error {
	imageFolderLink := getImageDir(cacheDir, imageRef)
	if skipCache && utils.PathExists(imageFolderLink) {
		return nil
	}

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

	os.MkdirAll(exportDir, 0755)
	tarFile := filepath.Join(exportDir, "fs.tar")
	if !utils.PathExists(tarFile) {
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

	// extract config
	configFilePath := filepath.Join(exportDir, "config.json")
	if !utils.PathExists(configFilePath) {
		data, err := img.RawConfigFile()
		if err != nil {
			return err
		}

		err = os.WriteFile(configFilePath, data, fs.ModePerm)
		if err != nil {
			return err
		}
	}

	if utils.PathExists(imageFolderLink) {
		// extract old symlink target. we should remove it and delete the old files
		oldPath, err := filepath.EvalSymlinks(imageFolderLink)
		if err != nil {
			return err
		}

		// if the symlink is the same as exportDir we can skipt
		if oldPath == exportDir {
			return nil
		}

		err = os.Remove(imageFolderLink)
		if err != nil {
			return err
		}

		err = os.RemoveAll(oldPath)
		if err != nil {
			return err
		}
	}

	// symlink imageRef -> imgId
	return os.Symlink(exportDir, imageFolderLink)
}

func prepContainer(imageRef, cacheDir, runDir string) (string, *v1.Config, error) {
	imgDir := getImageDir(cacheDir, imageRef)
	tarFile := filepath.Join(imgDir, "fs.tar")

	// extract filesystem
	instanceId := uuid.NewString()
	rootFS := filepath.Join(runDir, instanceId)
	os.MkdirAll(rootFS, 0755)
	err := utils.Untar(tarFile, rootFS)
	if err != nil {
		return "", nil, err
	}

	// load config
	cfgFilePath := filepath.Join(imgDir, "config.json")
	data, err := os.ReadFile(cfgFilePath)
	if err != nil {
		return "", nil, err
	}

	cfgFile := v1.ConfigFile{}
	err = json.Unmarshal(data, &cfgFile)
	if err != nil {
		return "", nil, err
	}

	return rootFS, &cfgFile.Config, nil
}

func getImageDir(cacheDir, imageRef string) string {
	imageRefHash := base64.StdEncoding.EncodeToString([]byte(imageRef))
	return filepath.Join(cacheDir, imageRefHash)
}

// reference from https://github.com/moby/moby/blob/master/pkg/reexec/command_linux.go
// Pdeathsig ensures the child receies SIGTERM if parent dies
func reexecCmd(args ...string) *exec.Cmd {
	return &exec.Cmd{
		Path:   "/proc/self/exe",
		Args:   args,
		Stdin:  os.Stdin,
		Stdout: os.Stdout,
		Stderr: os.Stderr,
		SysProcAttr: &syscall.SysProcAttr{
			Pdeathsig: unix.SIGTERM,
			// Cloneflags:   syscall.CLONE_NEWUTS | syscall.CLONE_NEWPID | syscall.CLONE_NEWNS,
			// Unshareflags: syscall.CLONE_NEWNS,
			Cloneflags: syscall.CLONE_NEWNS |
				syscall.CLONE_NEWUTS |
				syscall.CLONE_NEWIPC |
				syscall.CLONE_NEWPID |
				syscall.CLONE_NEWNET |
				syscall.CLONE_NEWUSER,
			UidMappings: []syscall.SysProcIDMap{
				{
					ContainerID: 0,
					HostID:      os.Getuid(),
					Size:        1,
				},
			},
			GidMappings: []syscall.SysProcIDMap{
				{
					ContainerID: 0,
					HostID:      os.Getgid(),
					Size:        1,
				},
			},
		},
	}
}

// Reference: https://github.com/teddyking/ns-process/blob/master/rootfs.go
func pivotRoot(newroot string) error {
	putold := filepath.Join(newroot, "/.pivot_root")

	// bind mount newroot to itself - this is a slight hack needed to satisfy the
	// pivot_root requirement that newroot and putold must not be on the same
	// filesystem as the current root
	if err := syscall.Mount(newroot, newroot, "", syscall.MS_BIND|syscall.MS_REC, ""); err != nil {
		return err
	}

	// create putold directory
	if err := os.MkdirAll(putold, 0700); err != nil {
		return err
	}

	// call pivot_root
	if err := syscall.PivotRoot(newroot, putold); err != nil {
		return err
	}

	// ensure current working directory is set to new root
	if err := os.Chdir("/"); err != nil {
		return err
	}

	// umount putold, which now lives at /.pivot_root
	putold = "/.pivot_root"
	if err := syscall.Unmount(putold, syscall.MNT_DETACH); err != nil {
		return err
	}

	// remove putold
	if err := os.RemoveAll(putold); err != nil {
		return err
	}

	return nil
}

func mountProc(newroot string) error {
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

func mountBind(source, target string) error {
	// create target directory
	if err := os.MkdirAll(target, 0700); err != nil {
		return err
	}

	return syscall.Mount(source, target, "", syscall.MS_BIND|syscall.MS_REC, "")
}

func mountTmpfs(path string, size int64) error {
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

// Initialize namespace
func nsInitialisation(rootFS string, cfg *v1.Config, bindMounts []BindMount, tmpfsMounts []TmpfsMount, cmdToRun []string) error {

	if err := mountProc(rootFS); err != nil {
		return err
	}

	if err := pivotRoot(rootFS); err != nil {
		return err
	}

	if cfg.Hostname != "" {
		if err := syscall.Sethostname([]byte(cfg.Hostname)); err != nil {
			return err
		}
	}

	for _, mt := range bindMounts {
		if err := mountBind(mt.Source, mt.Target); err != nil {
			return err
		}
	}

	for _, mt := range tmpfsMounts {
		if err := mountTmpfs(mt.Path, mt.Size); err != nil {
			return err
		}
	}

	args := cmdToRun
	if len(cmdToRun) == 0 {
		args = cfg.Cmd
	}

	if len(cfg.Entrypoint) != 0 {
		args = append(cfg.Entrypoint, args...)
	}

	if len(args) == 0 {
		return errors.New("no command to run")
	}

	return nsRun(args[0], args, cfg.Env)
}

// Run command in namespace
func nsRun(name string, args []string, env []string) error {
	cmd := exec.Cmd{
		Path:   name,
		Args:   args,
		Env:    env,
		Stdin:  os.Stdin,
		Stdout: os.Stdout,
		Stderr: os.Stderr,
		SysProcAttr: &syscall.SysProcAttr{
			Pdeathsig: unix.SIGTERM,
		},
	}

	return cmd.Run()
}
