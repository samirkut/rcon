package cmd

import (
	"github.com/spf13/cobra"
)

var (
	verboseLogging, quietLogging bool
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "rcon",
	Short: "Simple and limited container runner",
	Long: `A dead simple and somewhat naive container runner. 
	
	The focus is on creating a runtime which can work with older kernels where podman won't work. 
	This of course comes at a cost but in some cases the trade-off is probably worth it.`,
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	cobra.CheckErr(rootCmd.Execute())
}

func init() {
	rootCmd.PersistentFlags().BoolVarP(&verboseLogging, "verbose", "v", false, "enable verbose logging")
	rootCmd.PersistentFlags().BoolVarP(&quietLogging, "quiet", "q", false, "disable logging")
}
