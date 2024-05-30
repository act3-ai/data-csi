// Package docs package docs supports the auto generation of documentation for csi driver commands
package docs

import (
	"embed"

	"github.com/spf13/cobra"

	"gitlab.com/act3-ai/asce/go-common/pkg/embedutil"
)

//nolint:revive
//go:embed README.md
var README embed.FS

// Embedded returns the Layout of embedded documentation to surface in the help command
// and generate in the gendocs command.
func Embedded(root *cobra.Command) *embedutil.Documentation {
	return &embedutil.Documentation{
		Title:   "CSI Driver",
		Command: root,
		Categories: []*embedutil.Category{
			embedutil.NewCategory(
				"docs", "General Documentation", root.Name(), 1,
				embedutil.LoadMarkdown("readme", "README", "README.md", README),
			),
		},
	}
}
