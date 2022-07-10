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

var (
	logger = utils.MustGetLogger()

	cacheDir  string
	runDir    string
	authFile  string
	mounts    = []string{}
	skipCache bool
)

// runCmd represents the run command
var runCmd = &cobra.Command{
	Use:   "run image-path [command]",
	Short: "Run the container",
	Long:  `Run the container based on options passed in`,
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if verboseLogging {
			utils.SetLoggerVerbose()
		}

		if quietLogging {
			utils.SetLoggerQuiet()
		}

		runDir, err := utils.EnsureDir(runDir)
		if err != nil {
			return err
		}

		if runDir == "" {
			return errors.New("--run-dir is required")
		}

		cacheDir, err = utils.EnsureDir(cacheDir)
		if err != nil {
			return err
		}

		if cacheDir == "" {
			return errors.New("--cache-dir is required")
		}

		authFile, err = utils.ExpandPath(authFile)
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

		if os.Args[0] != "ns" {
			logger.Tracef("Forking with NS enabled")
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
			logger.Tracef("Unmounting %s", rootFS)
			_ = syscall.Unmount(rootFS, 0)
		}()

		// initialize namespace with mounts, hostname
		err = nsInitialisation(rootFS, cfg.Hostname, bindMounts, tmpfsMounts)
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

	runCmd.Flags().StringVar(&runDir, "run-dir", "~/.rcon/run", "cache folder for images")
	runCmd.Flags().StringVar(&cacheDir, "cache-dir", "~/.rcon/cache", "cache folder for images")
	runCmd.Flags().StringVar(&authFile, "auth-file", "~/.rcon/auth.json", "auth file for accessing container registry")
	runCmd.Flags().BoolVar(&skipCache, "skip-cache", false, "refetch image from server instead of using cache")
	runCmd.Flags().StringArrayVar(&mounts, "mount", nil, "mounts to pass in specified as host_path:container_path for bind mounts, or just container_path:tmpfs:size_bytes for tmpfs")
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
func nsInitialisation(rootFS string, hostname string, bindMounts []BindMount, tmpfsMounts []TmpfsMount) error {
	logger.Tracef("Initialize namespace and mounts")

	if err := container.MountProc(rootFS); err != nil {
		return err
	}

	if err := container.PivotRoot(rootFS); err != nil {
		return err
	}

	if hostname != "" {
		logger.Tracef("set container hostname to %s", hostname)
		if err := syscall.Sethostname([]byte(hostname)); err != nil {
			return err
		}
	}

	for _, mt := range bindMounts {
		if err := container.MountBind(mt.Source, mt.Target); err != nil {
			return err
		}
	}

	for _, mt := range tmpfsMounts {
		if err := container.MountTmpfs(mt.Path, mt.Size, false); err != nil {
			return err
		}
	}

	return nil
}

// Run command in namespace
func nsRun(name string, args []string, env []string) error {
	logger.Tracef("Launching command in container: %s (%v)", name, args)
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
