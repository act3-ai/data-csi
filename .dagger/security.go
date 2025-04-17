package main

import (
	"context"
	"dagger/csi/internal/dagger"
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

// Use ace-dt to perform a vulnerability scan on a list of OCI artifacts.
func (c *Csi) Scan(ctx context.Context,
	// Path to OCI artifact list
	sources *dagger.File,
) (string, error) {
	grype := dag.Container().
		From(imageGrype).
		File("/grype")

	grypeDB := c.GrypeDB(ctx)

	syft := dag.Container().
		From(imageSyft).
		File("/syft")

	const cachePath = "/cache/grype"

	acedt := dag.Container().
		From(imageAcedt).
		File("/usr/local/bin/ace-dt")

	sourcePath := "artifacts.txt"
	return dag.Container().
		WithMountedSecret("/root/.docker/config.json", c.RegistryConfig.Secret()).
		From("cgr.dev/chainguard/bash").
		WithFile("/usr/local/bin/ace-dt", acedt).
		WithFile("/usr/local/bin/grype", grype).
		WithFile("/usr/local/bin/syft", syft).
		WithFile(sourcePath, sources).
		WithDirectory(cachePath, grypeDB). // grype db update fails if mounted
		WithEnvVariable("GRYPE_DB_CACHE_DIR", cachePath).
		WithUser("0").
		WithExec([]string{"grype", "db", "update"}).
		WithExec([]string{"ace-dt", "security", "scan", "-o=table",
			"--source-file", sourcePath, "--push-reports"}).
		Stdout(ctx)
}

// Download the Grype vulnerability database
func (c *Csi) GrypeDB(ctx context.Context) *dagger.Directory {
	const cachePath = "/cache/grype"

	return dag.Container().
		From(imageGrype).
		// WithUser(owner).
		// WithMountedCache(cachePath, dag.CacheVolume("grype-db-cache"), dagger.ContainerWithMountedCacheOpts{Owner: owner}).
		// comment out the line below to see the cached date output
		// WithEnvVariable("CACHEBUSTER", time.Now().String()).
		WithEnvVariable("GRYPE_DB_CACHE_DIR", cachePath).
		WithExec([]string{"/grype", "db", "update"}).
		Directory(cachePath)
}
