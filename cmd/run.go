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
	"errors"
	"os"
	"os/exec"
	"rcon/container"
	"rcon/utils"
	"strconv"
	"strings"
	"syscall"

	v1 "github.com/google/go-containerregistry/pkg/v1"
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

			err := cmd.Run()
			if err != nil {
				// suppress help from being shown by returning nil
				// but lets propogate the exit code
				if exitErr, ok := err.(*exec.ExitError); ok {
					os.Exit(exitErr.ExitCode())
				}
			}
			return nil
		}

		// all the lines below run within a new namespace
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

		authFile, err := cmd.Flags().GetString("auth-file")
		if err != nil {
			return err
		}

		authFile, err = utils.ExpandPath(authFile)
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

		err = container.FetchContainer(imageRef, cacheDir, authFile, skipCache)
		if err != nil {
			return err
		}

		rootFS, cfg, err := container.PrepContainer(imageRef, cacheDir, runDir)
		if err != nil {
			return err
		}

		// clean up rootFS on exit
		defer func() {
			//log.Println("Removing", rootFS)
			_ = os.RemoveAll(rootFS)
		}()

		// initialize namespace with mounts, hostname
		err = nsInitialisation(rootFS, cfg, bindMounts, tmpfsMounts)
		if err != nil {
			return err
		}

		// run the command - ignore args[0] since thats the image ref
		cmdArgs := args[1:]
		if len(cmdArgs) == 0 {
			cmdArgs = cfg.Cmd
		}

		if len(cfg.Entrypoint) != 0 {
			cmdArgs = append(cfg.Entrypoint, cmdArgs...)
		}

		if len(cmdArgs) == 0 {
			return errors.New("no command to run")
		}

		return nsRun(cmdArgs[0], cmdArgs, cfg.Env)
	},
}

func init() {
	rootCmd.AddCommand(runCmd)

	runCmd.Flags().String("run-dir", "/tmp/rcon/run", "cache folder for images")
	runCmd.Flags().String("cache-dir", "~/.rcon/cache", "cache folder for images")
	runCmd.Flags().String("auth-file", "~/.rcon/auth", "auth file for accessing container registry")
	runCmd.Flags().Bool("skip-cache", false, "use image in cache if possible")
	runCmd.Flags().StringArray("mount", nil, "mounts to pass in specified as host_path:container_path for bind mounts, or just container_path:tmpfs:size_bytes for tmpfs")
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
				// syscall.CLONE_NEWNET |
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

// Initialize namespace
func nsInitialisation(rootFS string, cfg *v1.Config, bindMounts []BindMount, tmpfsMounts []TmpfsMount) error {

	if err := container.MountProc(rootFS); err != nil {
		return err
	}

	if err := container.PivotRoot(rootFS); err != nil {
		return err
	}

	if cfg.Hostname != "" {
		if err := syscall.Sethostname([]byte(cfg.Hostname)); err != nil {
			return err
		}
	}

	for _, mt := range bindMounts {
		if err := container.MountBind(mt.Source, mt.Target); err != nil {
			return err
		}
	}

	for _, mt := range tmpfsMounts {
		if err := container.MountTmpfs(mt.Path, mt.Size); err != nil {
			return err
		}
	}

	return nil
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
