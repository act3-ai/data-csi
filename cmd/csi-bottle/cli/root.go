// Package cli implements the command line interface for the CSI driver
package cli

import (
	"github.com/spf13/cobra"

	"github.com/act3-ai/go-common/pkg/version"
)

// NewRootCmd create a new root command.
func NewRootCmd(info version.Info) *cobra.Command {
	// rootCmd represents the base command when called without any subcommands
	cmd := &cobra.Command{
		Use:   "csi-bottle",
		Short: "Kubernetes CSI driver for creating volumes from data bottles",
		Long: `The driver implements the Identity and Node services for the container storage interface (CSI).
	This CSI driver s used to populate volumes from read-only data.`,
	}

	// add subcommands
	cmd.AddCommand(
		NewServeCmd(info),
		NewCheckPathCmd(),
	)

	return cmd
}
