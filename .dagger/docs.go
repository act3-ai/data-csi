package main

import (
	"context"
	"dagger/csi/internal/dagger"
)

// Generate CLI documentation.
func (c *Csi) CLIDocs(ctx context.Context) *dagger.Directory {
	csi := c.Build(ctx, "linux/amd64", false)

	cliDocsPath := "docs/cli"
	return dag.Go().
		WithSource(c.Source).
		Container().
		WithFile("/usr/local/bin/csi-bottle", csi).
		WithExec([]string{"csi-bottle", "gendocs", "md", "--only-commands", cliDocsPath}).
		Directory(cliDocsPath)
}
