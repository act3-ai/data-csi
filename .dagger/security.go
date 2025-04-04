package main

import (
	"context"
	"dagger/csi/internal/dagger"
)

// Run govulncheck.
func (c *Csi) VulnCheck(ctx context.Context) (string, error) {
	return dag.Go(
		dagger.GoOpts{
			Container: dag.Container().
				From(imageGo).
				WithMountedSecret("/root/.netrc", c.Netrc),
		}).
		WithSource(c.Source).
		WithCgoDisabled().
		Exec([]string{"go", "install", goVulnCheck}).
		WithExec([]string{"govulncheck", "./..."}).
		Stdout(ctx)
}
