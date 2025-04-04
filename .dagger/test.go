package main

import (
	"context"
	"dagger/csi/internal/dagger"
)

// Run tests.
func (c *Csi) Test() *Test {
	return &Test{
		Source:         c.Source,
		Netrc:          c.Netrc,
		RegistryConfig: c.RegistryConfig,
	}
}

// Test organizes test functions.
type Test struct {
	// source code directory
	// +defaultPath="/"
	Source *dagger.Directory

	// NETRC credentials
	// +private
	Netrc *dagger.Secret
	// +private
	RegistryConfig *dagger.RegistryConfig
}

// Run unit tests.
func (tt *Test) Unit(ctx context.Context) (string, error) {
	return dag.Go().
		WithSource(tt.Source).
		Container().
		WithExec([]string{"go", "test", "./..."}).
		Stdout(ctx)
}
