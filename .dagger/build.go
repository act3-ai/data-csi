package main

import (
	"context"
	"dagger/csi/internal/dagger"
	"fmt"
	"path"
	"strings"

	"github.com/sourcegraph/conc/pool"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"oras.land/oras-go/v2/registry"
)

// Generate a directory of csi-bottle executables built for all supported platforms, concurrently.
func (c *Csi) BuildPlatforms(ctx context.Context,
	// release version
	// +optional
	version string,
) (*dagger.Directory, error) {
	// build matrix
	gooses := []string{"linux"}
	goarches := []string{"amd64", "arm64", "arm", "s390x", "ppc64le"}

	ctx, span := Tracer().Start(ctx, "Build Platforms", trace.WithAttributes(attribute.StringSlice("GOOS", gooses), attribute.StringSlice("GOARCH", goarches)))
	defer span.End()

	buildsDir := dag.Directory()
	p := pool.NewWithResults[*dagger.File]().WithContext(ctx)

	for _, goos := range gooses {
		for _, goarch := range goarches {
			p.Go(func(ctx context.Context) (*dagger.File, error) {
				platform := fmt.Sprintf("%s/%s", goos, goarch)
				bin := c.Build(ctx, dagger.Platform(platform), version, "latest")
				return bin, nil
			})
		}
	}

	bins, err := p.Wait()
	if err != nil {
		return nil, err
	}
	return buildsDir.WithFiles(".", bins), nil
}

// Build an executable for the specified platform, named "csi-bottle-v{VERSION}-{GOOS}-{GOARCH}".
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
	// Release version, included in file name
	// +optional
	version string,
	// value of GOFIPS140, accepts modes "off", "latest", and "v1.0.0"
	// +optional
	// +default="latest"
	fipsMode string,
) *dagger.File {
	return build(ctx, c.Source, c.Netrc, platform, version, fipsMode)
}

func build(ctx context.Context,
	src *dagger.Directory,
	netrc *dagger.Secret,
	platform dagger.Platform,
	version string,
	fipsMode string,
) *dagger.File {
	// only name the result "fips" if it
	name := binaryName(string(platform), version)

	_, span := Tracer().Start(ctx, fmt.Sprintf("Build %s", name))
	defer span.End()

	return dag.Go().
		WithSource(src).
		WithCgoDisabled().
		WithEnvVariable("GOFIPS140", fipsMode).
		Build(dagger.GoWithSourceBuildOpts{
			Pkg:      "./cmd/csi-bottle",
			Platform: platform,
			Ldflags:  []string{"-s", "-w", fmt.Sprintf("-X 'main.version=%s'", version), "-extldflags 'static'"},
			Trimpath: true,
		}).
		WithName(name)
}

// binaryName constructs the name of a csi-bottle executable based on build params.
// All arguments are optional, building up to "telemetry-v{VERSION}-fips-{GOOS}-{GOARCH}".
func binaryName(platform string, version string) string {
	str := strings.Builder{}
	str.Grow(35) // est. max = len("telemetry-v1.11.11-fips-linux-amd64")
	str.WriteString("csi-bottle")

	if version != "" {
		str.WriteString("-v")
		str.WriteString(version)
	}

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
		WithFile("/usr/local/bin/csi-bottle", c.Build(ctx, platform, version, "latest")).
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
