package main

import (
	"context"
	"dagger/csi/internal/dagger"
	"fmt"
	"path"
	"strings"

	"github.com/sourcegraph/conc/pool"
	"oras.land/oras-go/v2/registry"
)

// Generate a directory of csi-bottle executables built for all supported platforms, concurrently.
func (c *Csi) BuildPlatforms(ctx context.Context,
	// snapshot build, skip goreleaser validations
	// +optional
	snapshot bool,
) *dagger.Directory {
	return GoReleaser(c.Source).
		WithExec([]string{"goreleaser", "build", "--clean", "--auto-snapshot", "--timeout=10m", fmt.Sprintf("--snapshot=%v", snapshot)}).
		Directory("dist")
}

// Build an executable for the specified platform, named "csi-bottle--{GOOS}-{GOARCH}".
//
// Supported Platform Matrix:
//
//	GOOS: linux
//	GOARCH: amd64, arm64, arm, s390x, ppc64le
func (c *Csi) Build(ctx context.Context,
	// Build target platform
	// +optional
	// +default="linux/amd64"
	platform dagger.Platform,
	// snapshot build, skip goreleaser validations
	// +optional
	snapshot bool,
) *dagger.File {
	return build(ctx, c.Source, platform, snapshot)
}

func build(ctx context.Context,
	src *dagger.Directory,
	platform dagger.Platform,
	// snapshot build, skip goreleaser validations
	snapshot bool,
) *dagger.File {
	name := binaryName(string(platform))

	_, span := Tracer().Start(ctx, fmt.Sprintf("Build %s", name))
	defer span.End()

	os, arch, _ := strings.Cut(string(platform), "/")
	return GoReleaser(src).
		WithEnvVariable("GOOS", os).
		WithEnvVariable("GOARCH", arch).
		WithExec([]string{"goreleaser", "build", "--auto-snapshot", "--timeout=10m", "--single-target", "--output", name, fmt.Sprintf("--snapshot=%v", snapshot)}).
		File(name)
}

// binaryName constructs the name of a csi-bottle executable based on build params.
// All arguments are optional, building up to "csi-bottle-{GOOS}-{GOARCH}".
func binaryName(platform string) string {
	str := strings.Builder{}
	str.WriteString("csi-bottle")

	if platform != "" {
		platform = strings.ReplaceAll(string(platform), "/", "-")
		str.WriteString("-")
		str.WriteString(platform)
	}

	return str.String()
}

// Create and publish a multi-platform image index.
func (c *Csi) ImageIndex(ctx context.Context,
	// image version
	version string,
	// OCI Reference
	address string,
	// build platforms
	platforms []dagger.Platform,
) (string, error) {
	ref, err := registry.ParseReference(address)
	if err != nil {
		return "", fmt.Errorf("parsing address: %w", err)
	}
	imgURL := "https://" + path.Join(ref.Registry, ref.Repository)
	// i := imageURL.
	p := pool.NewWithResults[*dagger.Container]().WithContext(ctx)
	for _, platform := range platforms {
		p.Go(func(ctx context.Context) (*dagger.Container, error) {
			img := c.Image(ctx, version, platform).
				WithLabel("org.opencontainers.image.url", imgURL).
				WithLabel("org.opencontainers.image.source", "https://github.com/act3-ai/data-csi")
			return img, nil
		})
	}

	platformVariants, err := p.Wait()
	if err != nil {
		return "", fmt.Errorf("building images: %w", err)
	}

	return dag.Container().
		Publish(ctx, address, dagger.ContainerPublishOpts{
			PlatformVariants: platformVariants,
		})
}

func (c *Csi) Image(ctx context.Context,
	// Image version
	version string,
	// Build target platform
	// +optional
	// +default="linux/amd64"
	platform dagger.Platform,
) *dagger.Container {
	// ensure to copy files, not mount them; else they won't be in the final image
	ctr := dag.Container(dagger.ContainerOpts{Platform: platform}).
		From(imageDistrolessDebian).
		WithFile("/usr/local/bin/csi-bottle", c.Build(ctx, platform, false)).
		WithEntrypoint([]string{"csi-bottle"}).
		WithWorkdir("/").
		WithLabel("description", "CSI Driver - For ASCE Data Bottles")

	return withCommonLabels(ctr, version)
}

// withCommonLabels applies common labels to a container, e.g. maintainers, vendor, etc.
func withCommonLabels(ctr *dagger.Container, version string) *dagger.Container {
	return ctr.
		WithLabel("maintainers", "Nathan D. Joslin <nathan.joslin@udri.udayton.edu>").
		WithLabel("org.opencontainers.image.vendor", "AFRL ACT3").
		WithLabel("org.opencontainers.image.version", version).
		WithLabel("org.opencontainers.image.title", "CSI Driver").
		WithLabel("org.opencontainers.image.url", "ghcr.io/act3-ai/data-csi").
		WithLabel("org.opencontainers.image.description", "ACE Data Tool Telemetry Server").
		WithLabel("org.opencontainers.image.source", "https://github.com/act3-ai/data-csi")
}
