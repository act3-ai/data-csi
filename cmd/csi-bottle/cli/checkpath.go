package cli

import (
	"os"

	"github.com/spf13/cobra"
)

// NewCheckPathCmd creates a new "serve" subcommand.
func NewCheckPathCmd() *cobra.Command {

	var cmd = &cobra.Command{
		Use:   "checkpath path",
		Short: "Checks that a path exists for testing purposes.",
		Long: `The command is to run to check a given path by csi-sanity. 
It prints 'file', 'directory', 'not_found', or 'other' on stdout.`,
		Args:   cobra.ExactArgs(1),
		Hidden: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SetOut(os.Stdout)
			pk, err := checkPath(args[0])
			if err != nil {
				return err
			}
			cmd.Println(pk)
			return nil
		},
	}

	return cmd
}

func checkPath(path string) (string, error) {
	fi, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "not_found", nil
		}
		return "", err
	}
	var pk string
	switch mode := fi.Mode(); {
	case mode.IsRegular():
		pk = "file"
	case mode.IsDir():
		pk = "directory"
	default:
		pk = "other"
	}
	return pk, nil
}
