// A generated module for Csi functions
//
// This module has been generated via dagger init and serves as a reference to
// basic module structure as you get started with Dagger.
//
// Two functions have been pre-created. You can modify, delete, or add to them,
// as needed. They demonstrate usage of arguments and return cypes using simple
// echo and grep commands. The functions can be called from the dagger CLI or
// from one of the SDKs.
//
// The first line in this comment block is a short description line and the
// rest is a long description with more detail on the module's purpose or usage,
// if appropriate. All modules should have a short description.

package main

import (
	"dagger/csi/internal/dagger"
)

const (
	// repository information
	gitRepo = "act3-ai/data-csi"

	// images
	imageGitCliff         = "docker.io/orhunp/git-cliff:2.8.0"
	imageDistrolessDebian = "gcr.io/distroless/static-debian12:debug"
	imageGrype            = "anchore/grype:latest"
	imageSyft             = "anchore/syft:latest"
	imageAcedt            = "ghcr.io/act3-ai/data-tool:v1.15.33"

	// go tools
	goVulnCheck = "golang.org/x/vuln/cmd/govulncheck@latest"
)

type Csi struct {
	// source code directory
	Source *dagger.Directory

	// +private
	RegistryConfig *dagger.RegistryConfig
	// +private
	Netrc *dagger.Secret
}

func New(
	// top level source code directory
	// +defaultPath="/"
	src *dagger.Directory,
) *Csi {
	return &Csi{
		Source:         src,
		RegistryConfig: dag.RegistryConfig(),
	}
}

// Add credentials for a registry.
func (c *Csi) WithRegistryAuth(
	// registry's hostname
	address string,
	// username in registry
	username string,
	// password or token for registry
	secret *dagger.Secret,
) *Csi {
	c.RegistryConfig = c.RegistryConfig.WithRegistryAuth(address, username, secret)
	return c
}

// Removes credentials for a registry.
func (c *Csi) WithoutRegistryAuth(
	// registry's hostname
	address string,
) *Csi {
	c.RegistryConfig = c.RegistryConfig.WithoutRegistryAuth(address)
	return c
}

// Add netrc credentials for a private git repository.
func (c *Csi) WithNetrc(
	// NETRC credentials
	netrc *dagger.Secret,
) *Csi {
	c.Netrc = netrc
	return c
}
