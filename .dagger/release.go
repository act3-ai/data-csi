package main

import (
	"context"
	"dagger/csi/internal/dagger"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Run release steps.
func (c *Csi) Release(
	// top level source code directory
	// +defaultPath="/"
	src *dagger.Directory,
	// gitlab token
	// +optional
	token *dagger.Secret,
) *Release {
	return &Release{
		Source: src,
		Token:  token,
	}
}

// Release provides utilties for preparing and publishing releases
// with git-cliff.
type Release struct {
	// source code directory
	// +defaultPath="/"
	Source *dagger.Directory

	// GitLab token
	// +optional
	Token *dagger.Secret
}

// Update the changelog, release notes, version, and helm chart versions.
func (r *Release) Prepare(ctx context.Context) (*dagger.Directory, error) {
	changelog := r.Changelog(ctx)
	version, err := r.Version(ctx)
	if err != nil {
		return nil, err
	}

	// must update chart version after bumping release version, and
	// before building notes
	chartFile, valuesFile := r.setChartVersion(version)

	notes, err := r.Notes(ctx, version)
	if err != nil {
		return nil, err
	}

	notesPath := filepath.Join("releases", fmt.Sprintf("v%s.md", version))
	return dag.Directory().
			WithFile("CHANGELOG.md", changelog).
			WithNewFile("VERSION", version+"\n").
			WithNewFile(notesPath, notes).
			WithFile(chartPath, chartFile).
			WithFile(chartValuesPath, valuesFile),
		nil
}

// Publish the current release. This should be tagged.
func (r *Release) Publish(ctx context.Context,
	// source code directory
	// +defaultPath="/"
	src *dagger.Directory,
	// github personal access token
	token *dagger.Secret,
	// commit ssh private key
	sshPrivateKey *dagger.Secret,
	// releaser username
	author string,
	//releaser email
	email string,
	// tag release as latest
	// +default=true
	// +optional
	latest bool,
) (string, error) {
	version, err := src.File("VERSION").Contents(ctx)
	if err != nil {
		return "", err
	}
	version = strings.TrimSpace(version)
	vVersion := "v" + version

	notesPath := filepath.Join("releases", vVersion+".md")
	return GoReleaser(src).
		WithSecretVariable("GITHUB_TOKEN", token).
		WithSecretVariable("SSH_PRIVATE_KEY", sshPrivateKey).
		WithEnvVariable("RELEASE_AUTHOR", author).
		WithEnvVariable("RELEASE_AUTHOR_EMAIL", email).
		WithEnvVariable("RELEASE_LATEST", strconv.FormatBool(latest)).
		WithExec([]string{"goreleaser", "release", "--fail-fast", "--release-notes", notesPath}).
		Stdout(ctx)
}

// Generate the change log from conventional commit messages (see cliff.toml)
func (r *Release) Changelog(ctx context.Context) *dagger.File {
	const changelogPath = "/app/CHANGELOG.md"
	return r.gitCliffContainer().
		WithExec([]string{"git-cliff", "--bump", "--strip=footer", "--unreleased", "--prepend", changelogPath}).
		File(changelogPath)
}

// Generate the next version from conventional commit messages (see cliff.toml)
func (r *Release) Version(ctx context.Context) (string, error) {
	version, err := r.gitCliffContainer().
		WithExec([]string{"git-cliff", "--bumped-version"}).
		Stdout(ctx)
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(version)[1:], err
}

// Generate the initial release notes
func (r *Release) Notes(ctx context.Context,
	// release version
	version string,
) (string, error) {
	notes, err := r.gitCliffContainer().
		WithExec([]string{"git-cliff", "--bump", "--unreleased", "--strip=all"}).
		Stdout(ctx)
	if err != nil {
		return "", err
	}

	// TODO: The helm chart is tagged as '1.2.3' while images are tagged 'v1.2.3', this is a
	// legacy release process convention that we may want to change.
	b := &strings.Builder{}
	b.WriteString("| Charts |\n")
	b.WriteString("| ----------------------------------------------------- |\n")
	fmt.Fprintf(b, "| ghcr.io/act3-ai/data-csi/charts/csi-bottle:%s |\n\n", version)

	b.WriteString("| Images |\n")
	b.WriteString("| --------------------------------------------------------- |\n")
	fmt.Fprintf(b, "| ghcr.io/act3-ai/data-csi/csi-bottle:%s |\n\n", "v"+version)

	b.WriteString("### ")
	notes = strings.Replace(notes, "### ", b.String(), 1)

	return notes, nil
}

func (r *Release) gitCliffContainer() *dagger.Container {
	return dag.Container().
		From(imageGitCliff).
		With(func(c *dagger.Container) *dagger.Container {
			if r.Token != nil {
				return c.WithSecretVariable("GITHUB_TOKEN", r.Token).
					WithEnvVariable("GITHUB_REPO", gitRepo)
			}
			return c
		}).
		WithMountedDirectory("/app", r.Source)
}

// GoReleaser provides a container with go-releaser, inheriting
// GOMAXPROCS and GOMEMLIMIT from the host environment.
func GoReleaser(src *dagger.Directory) *dagger.Container {
	ctr := dag.Container().
		From(imageGoReleaser).
		WithMountedCache("csi-bottle-cache", dag.CacheVolume("csi-bottle-cache")).
		WithMountedDirectory("/work/src", src).
		WithWorkdir("/work/src")

	goMaxProcs, ok := os.LookupEnv("GOMAXPROCS")
	if ok {
		ctr = ctr.WithEnvVariable("GOMAXPROCS", goMaxProcs)
	}
	goMemLimit, ok := os.LookupEnv("GOMEMLIMIT")
	if ok {
		ctr = ctr.WithEnvVariable("GOMEMLIMIT", goMemLimit)
	}

	return ctr
}
