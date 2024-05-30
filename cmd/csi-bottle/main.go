// Package main is the main program for the CSI bottle driver.
package main

import (
	"context"
	"log/slog"
	"os"

	"github.com/spf13/cobra"

	commands "gitlab.com/act3-ai/asce/go-common/pkg/cmd"
	"gitlab.com/act3-ai/asce/go-common/pkg/logger"
	"gitlab.com/act3-ai/asce/go-common/pkg/runner"
	vv "gitlab.com/act3-ai/asce/go-common/pkg/version"

	docs "gitlab.com/act3-ai/asce/data/csi"
	"gitlab.com/act3-ai/asce/data/csi/cmd/csi-bottle/cli"
)

// Retrieves build info.
func getVersionInfo() vv.Info {
	info := vv.Get()
	if version != "" {
		info.Version = version
	}
	return info
}

func main() {
	info := getVersionInfo()     // Load the version info from the build
	root := cli.NewRootCmd(info) // Create the root command
	root.SilenceUsage = true     // Silence usage when root is called

	handler := runner.SetupLoggingHandler(root, "ACE_DATA_CSI_VERBOSITY") // create log handler
	l := slog.New(handler)
	ctx := logger.NewContext(context.Background(), l)
	root.SetContext(ctx) // inject context for data commands

	// Layout of embedded documentation to surface in the help command
	// and generate in the gendocs command
	embedDocs := docs.Embedded(root)

	// Add common commands
	root.AddCommand(
		commands.NewVersionCmd(info),
		commands.NewGendocsCmd(embedDocs),
		commands.NewInfoCmd(embedDocs),
	)

	// Store persistent pre run function to avoid overwriting it
	persistentPreRun := root.PersistentPreRun

	// The pre run function logs build info and sets the default output writer
	root.PersistentPreRun = func(cmd *cobra.Command, args []string) {
		log := logger.FromContext(cmd.Context()) //nolint:contextcheck
		log.InfoContext(ctx, "Software", "version", info.Version)
		log.InfoContext(ctx, "Software details", "info", info)

		if persistentPreRun != nil {
			persistentPreRun(cmd, args)
		}
	}

	// Run the root command
	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}
