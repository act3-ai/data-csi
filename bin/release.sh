#!/usr/bin/env bash

set -euo pipefail

help() {
    cat <<EOF
Try using one of the following commands:
prepare - prepare a release locally by producing the changelog, notes, assets, etc.
approve - commit, tag, and push your approved release.
publish - publish the release to GitLab by uploading assets, images, helm chart, etc.

Dependencies: dagger, oras, make, and git.

Required Environment Variables:
Without Defaults:
    - GITHUB_API_TOKEN - repo access
    - GITHUB_REG_TOKEN - write:packages access
    - GITHUB_REG_USER  - username of GITHUB_REG_TOKEN owner

EOF
    exit 1
}

if [ "$#" != "1" ]; then
    help
fi

set -x

registry=ghcr.io
registryRepo=$registry/act3-ai/data-csi

# Extract the major version of a release.
parseMajor() {
    echo -n "$(echo -n "$1" | sed 's/v//' | cut -d "." -f 1)"
}

# Extract the minor version of a release.
parseMinor() {
    echo -n "$(echo -n "$1" | sed 's/v//' | cut -d "." -f 2)"
}

# Extract the patch version of a release.
parsePatch() {
    echo -n "$(echo -n "$1" | sed 's/v//' | cut -d "." -f 3)"
}

# Determines if the target version is the latest patch release of all releases
# with the same major and minor version.
isLatestPatch() {
    allTags="$1"
    targetMajor="$2"
    targetMinor="$3"
    targetPatch="$4"

    sameMajorMinors=$(echo "$allTags" | grep -P "^v$targetMajor\.$targetMinor\.\d+$")

    result="true"
    for v in $sameMajorMinors
    do
        if [ "$(parsePatch "$v")" -gt "$targetPatch" ]; then
            result="false"
            break
        fi
    done

    echo -n "$result"
}

# Determines if the target version is the latest minor release of all releases
# with the same major version.
isLatestMinor() {
    allTags="$1"
    targetMajor="$2"
    targetMinor="$3"

    sameMajors=$(echo "$allTags" | grep -P "^v$targetMajor\.\d+\.\d+$")

    result="true"
    for v in $sameMajors
    do
        if [ "$(parseMinor "$v")" -gt "$targetMinor" ]; then
            result="false"
            break
        fi
    done

    echo -n "$result"
}

# Determines if the target version is the latest major release.
isLatestMajor() {
    allTags="$1"
    targetMajor="$2"

    allFullTags=$(echo "$allTags" | grep -P "^v\d+\.\d+\.\d+$")

    result="true"
    for v in $allFullTags
    do
        if [ "$(parseMajor "$v")" -gt "$targetMajor" ]; then
            result="false"
            break
        fi
    done

    echo -n "$result"
}

# Determine extra tags based on existing release tags, e.g. should release v1.2.3
# also tag images for v1.2, v1, and latest. It does not check if a tag already
# exists. Only considers tags of the form '^v\d+\.\d+\.\d+$', e.g. beta releases
# are excluded.
# Input: OCI repository reference, without tag.
# Output: space separated list of tags, as a string.
resolveExtraTags() {
    ref="$1"
    targetVersion="$2"

    allTags=$(oras repo tags --exclude-digest-tags "$ref" | grep -P "^v\d+\.\d+\.\d+$" | sort -Vr)

    targetMajor=$(parseMajor "$targetVersion")
    targetMinor=$(parseMinor "$targetVersion")
    targetPatch=$(parsePatch "$targetVersion")

    latestPatch=$(isLatestPatch "$allTags" "$targetMajor" "$targetMinor" "$targetPatch")
    latestMinor=$(isLatestMinor "$allTags" "$targetMajor" "$targetMinor")
    latestMajor=$(isLatestMajor "$allTags" "$targetMajor")

    extraTags=""

    # if latest patch (for the same major.minor releases), add "vX.X" tag
    if [ "$latestPatch" = "true"  ]; then
        extraTags="v${targetMajor}.${targetMinor}"
        # if also latest minor (for the same major releases), add "vX" tag
        if [ "$latestMinor" = "true" ]; then
            extraTags="$extraTags v${targetMajor}"
            # if also latest major add "latest" tag
            if [ "$latestMajor" = "true" ]; then
                extraTags="$extraTags latest"
            fi
        fi
    fi

    echo -n "$extraTags"
}

case $1 in
prepare)
    if [[ $(git diff --stat) != '' ]]; then
        echo 'Git repo is dirty, aborting'
        exit 2
    fi

    dagger call lint all

    # run unit tests (no other tests exist, besides testing with a VM)
    dagger call \
        test unit

    # update changelog, release notes, semantic version
    dagger call release prepare export --path=.

    # test the updated chart, after it's updated by release prepare
    dagger call test chart

    # govulncheck
    dagger call \
        vuln-check

    # generate CLI docs
    dagger call \
        clidocs \
        export --path=./docs/cli

    version=$(cat VERSION)

    echo "Please review the local changes, especially releases/$version.md"
    ;;

approve)
    version=v$(cat VERSION)
    notesPath="releases/$version.md"
    # release material
    git add VERSION CHANGELOG.md "$notesPath"
    # helm chart version bump
    git add charts/csi-bottle/Chart.yaml
    git add charts/csi-bottle/values.yaml
    # documentation changes (from make gendoc, apidoc, swagger)
    git add \*.md # updated
    # signed commit
    git commit -S -m "chore(release): prepare for $version"
    # annotated and signed tag
    git tag -s -a -m "Official release $version" "$version"
    # push this branch and the associated tags
    git push --follow-tags
    ;;

publish)
    version=$(cat VERSION)
    platforms=linux/amd64,linux/arm64,linux/arm,linux/s390x,linux/ppc64le

    # build for all supported platforms
    assetsDir=bin/release/assets # changes to this path must be reflected in .dagger/release.go Publish()
    mkdir -p "$assetsDir"
    dagger call \
        build-platforms --version="$version" \
        export --path="$assetsDir"

    # publish release
    dagger call \
        with-registry-auth --address="$registry" --username="$GITHUB_REG_USER" --secret=env:GITHUB_REG_TOKEN \
        release \
        publish --token=env:GITHUB_API_TOKEN

    # publish helm chart
    dagger call \
        release \
        publish-chart --ociRepo oci://$registryRepo/charts --address="$registry" --username="$GITHUB_REG_USER" --secret env:GITHUB_REG_TOKEN

    # publish image
    # Note: Changes to existing or inclusions of additional image references should be reflected in the release notes generated in ../.dagger/release.go
    imageRepoRef="${registryRepo}:${fullVersion}"
    dagger call \
        with-registry-auth --address=$registry --username="$GITHUB_REG_USER" --secret=env:GITHUB_REG_TOKEN \
        image-index --version="$fullVersion" --platforms="$platforms" --address "$imageRepoRef"

    # shellcheck disable=SC2046
    oras tag "$(oras discover "$imageRepoRef" | head -n 1)" $(resolveExtraTags "$registryRepo" "$fullVersion")

    # scan images with ace-dt
    echo "$imageRepoRef" > artifacts.txt
    # TODO: Uncomment me when we have a suitable public registry for custom artifact types.
    # dagger call with-registry-auth --address=$registry --username="$GITHUB_REG_USER" --secret=env:GITHUB_REG_TOKEN with-netrc --netrc=file:"$netrcPath" scan --sources artifacts.txt
    ;;

*)
    help
    ;;
esac
