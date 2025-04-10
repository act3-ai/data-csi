package main

import (
	"context"
)

// Run govulncheck.
func (c *Csi) VulnCheck(ctx context.Context) (string, error) {
	return dag.Go().
		WithSource(c.Source).
		WithCgoDisabled().
		Exec([]string{"go", "install", goVulnCheck}).
		WithExec([]string{"govulncheck", "./..."}).
		Stdout(ctx)
}
