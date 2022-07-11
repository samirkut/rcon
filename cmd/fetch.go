package cmd

import (
	"errors"
	"rcon/container"
	"rcon/utils"

	"github.com/spf13/cobra"
)

// fetchCmd represents the fetch command
var fetchCmd = &cobra.Command{
	Use:   "fetch image-path",
	Short: "Fetch the provided container ref and store it in cache",
	Long: `This command can be used to proactively fetch a certain image and store it in cache. 
	This helps make the container run happen much faster`,
	RunE: func(cmd *cobra.Command, args []string) error {
		var err error

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

		imageRef := args[0]

		return container.FetchContainer(imageRef, cacheDir, authFile, true)
	},
}

func init() {
	rootCmd.AddCommand(fetchCmd)

	fetchCmd.Flags().StringVar(&cacheDir, "cache-dir", "~/.rcon/cache", "cache folder for images")
	fetchCmd.Flags().StringVar(&authFile, "auth-file", "~/.rcon/auth.json", "auth file (json) for accessing container registry")
}
