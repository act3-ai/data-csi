package main

import (
	"context"
	"dagger/csi/internal/dagger"
	"path/filepath"
)

var (
	chartPath       = filepath.Join("charts", "csi-bottle", "Chart.yaml")
	chartValuesPath = filepath.Join("charts", "csi-bottle", "values.yaml")
)

// Run helm lint on the telemetry chart.
func (tt *Test) Chart(ctx context.Context) (string, error) {
	return dag.Helm().
		Lint(tt.Source.Directory(filepath.Dir(chartPath))).
		Stdout(ctx)
}

// Returns chart.yaml and values.yaml files with updated versions, respectively.
func (r *Release) setChartVersion(
	// release version
	version string,
) (*dagger.File, *dagger.File) {
	updatedChart := dag.Wolfi().
		Container(dagger.WolfiContainerOpts{
			Packages: []string{"yq"},
		}).
		WithMountedDirectory("/src", r.Source).
		WithWorkdir("/src").
		WithEnvVariable("version", version).
		WithExec([]string{"yq", "e",
			"(.version = env(version)) | (.appVersion = env(version))",
			"-i", chartPath}).
		File(chartPath)

	updatedValues := dag.Wolfi().Container(dagger.WolfiContainerOpts{
		Packages: []string{"yq"},
	}).
		WithMountedDirectory("/src", r.Source).
		WithWorkdir("/src").
		WithEnvVariable("version", version).
		WithExec([]string{"yq", "e",
			".image.tag = \"v\" + env(version)",
			"-i", chartValuesPath}).
		File(chartValuesPath)

	return updatedChart, updatedValues
}

// Publish the helm chart.
func (r *Release) PublishChart(ctx context.Context,
	// OCI Ref, without tag
	ociRepo string,
	// registry's hostname
	address string,
	// username in registry
	username string,
	// password or token for registry
	secret *dagger.Secret,
) error {
	return dag.Helm().
		WithRegistryAuth(address, username, secret).
		Chart(r.Source.Directory(filepath.Dir(chartPath))).
		Package(dagger.HelmChartPackageOpts{DependencyUpdate: true}).
		Publish(ctx, ociRepo)
}
