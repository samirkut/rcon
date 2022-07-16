package cmd

import (
	"fmt"
	"rcon/container"
	"rcon/utils"
	"syscall"

	"github.com/spf13/cobra"
	"golang.org/x/term"
)

var (
	username string
)

// loginCmd represents the login command
var loginCmd = &cobra.Command{
	Use:   "login server-url",
	Short: "Register username/secret for the specified registry",
	Long:  `Persists login information to the specified auth file`,
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		var err error

		authFile, err = utils.ExpandPath(authFile)
		if err != nil {
			return err
		}

		if username == "" {
			// assume this is a token, so we set the username to special string <token>
			// Reference: https://github.com/google/go-containerregistry/blob/main/pkg/authn/README.md
			username = "<token>"
		}

		fmt.Print("Secret: ")
		bytePassword, err := term.ReadPassword(int(syscall.Stdin))
		if err != nil {
			return err
		}

		serverUrl := args[0]

		helper := &container.AuthHelper{AuthFile: authFile}

		return helper.Add(serverUrl, username, string(bytePassword))
	},
}

func init() {
	rootCmd.AddCommand(loginCmd)

	loginCmd.Flags().StringVar(&username, "username", "", "specify username. leave blank for tokens")
	loginCmd.Flags().StringVar(&authFile, "auth-file", "~/.rcon/auth.json", "auth file (json) for accessing container registry")
}
