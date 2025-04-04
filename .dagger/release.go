package main

import (
	"context"
	"dagger/csi/internal/dagger"
	"fmt"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"dagger.io/dagger/dag"
	"github.com/sourcegraph/conc/pool"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
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
	// gitlab personal access token
	token *dagger.Secret,
) (string, error) {
	version, err := src.File("VERSION").Contents(ctx)
	if err != nil {
		return "", err
	}
	version = strings.TrimSpace(version)

	notesFileName := fmt.Sprintf("v%s.md", version)
	notes := src.File(filepath.Join("releases", notesFileName))

	return r.createRelease(ctx, version, notes, token)
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
	fmt.Fprintf(b, "| registry.gitlab.com/ace/data/csi/charts/csi-bottle:%s |", version)

	b.WriteString("| Images |\n")
	b.WriteString("| --------------------------------------------------------- |\n")
	fmt.Fprintf(b, "| registry.gitlab.com/ace/data/csi/csi-bottle:%s |\n", "v"+version)

	b.WriteRune('\n')
	b.WriteString("### ")

	notes = strings.Replace(notes, "### ", b.String(), 1)

	return notes, nil
}

// create a release for an existing tag.
func (r *Release) createRelease(ctx context.Context,
	// release version
	version string,
	// release notes file
	notes *dagger.File,
	// gitlab personal access token
	token *dagger.Secret,
) (string, error) {
	notesFileName, err := notes.Name(ctx)
	if err != nil {
		return "", err
	}
	return dag.Container().
		From(imageGitlabCLI).
		WithMountedFile(notesFileName, notes).
		WithSecretVariable("GITLAB_TOKEN", token).
		WithEnvVariable("GITLAB_HOST", gitlabHost).
		WithExec([]string{"glab", "release", "create",
			"-R", gitlabProject, // repository
			"v" + version,                 // tag
			"--name=Release v" + version,  // title
			"--notes-file", notesFileName, // description
		}).
		Stdout(ctx)
}

func (r *Release) gitCliffContainer() *dagger.Container {
	return dag.Container().
		From(imageGitCliff).
		With(func(c *dagger.Container) *dagger.Container {
			if r.Token != nil {
				return c.WithSecretVariable("GITLAB_TOKEN", r.Token).
					WithEnvVariable("GITLAB_API_URL", path.Join(gitlabHost, "/api/v4")).
					WithEnvVariable("GITLAB_REPO", gitlabProject)
			}
			return c
		}).
		WithMountedDirectory("/app", r.Source)
}

// UploadAssets publishes binaries as assets to an existing release tag.
func (r *Release) UploadAssets(ctx context.Context,
	// release version
	version string,
	// release assets
	assets *dagger.Directory,
	// gitlab personal access token
	token *dagger.Secret,
) (string, error) {
	releaseAssets, err := assets.Entries(ctx)
	if err != nil {
		return "", err
	}

	ctx, span := Tracer().Start(ctx, "Upload Builds", trace.WithAttributes(attribute.StringSlice("Assets", releaseAssets)))
	defer span.End()

	// remove unwanted items that exist in bin dir
	wantedBins := regexp.MustCompile("csi-bottle-*")
	cleanedAssets := slices.DeleteFunc(releaseAssets, func(s string) bool {
		return !wantedBins.MatchString(s)
	})

	p := pool.NewWithResults[string]().WithContext(ctx)
	for _, asset := range cleanedAssets {
		p.Go(func(ctx context.Context) (string, error) {
			_, err := r.uploadBuild(ctx, version, assets.File(asset), token)
			if err != nil {
				return fmt.Sprintf("Failed to upload asset - %s", asset), err
			}
			return fmt.Sprintf("Asset Uploaded - %s", asset), nil
		})
	}

	result, err := p.Wait()
	return strings.Join(result, "\n"), err
}

func (r *Release) uploadBuild(ctx context.Context,
	// release version
	version string,
	// build file
	build *dagger.File,
	// gitlab personal access token
	token *dagger.Secret,
) (string, error) {
	buildName, err := build.Name(ctx)
	if err != nil {
		return "", err
	}
	ctx, span := Tracer().Start(ctx, fmt.Sprintf("upload release asset %s", buildName))
	defer span.End()

	return dag.Container().
		From(imageGitlabCLI).
		WithMountedFile(buildName, build).
		WithSecretVariable("GITLAB_TOKEN", token).
		WithEnvVariable("GITLAB_HOST", gitlabHost).
		WithExec([]string{"glab", "release", "upload",
			"-R", gitlabProject,
			"v" + version,
			buildName},
		).
		Stdout(ctx)
}
