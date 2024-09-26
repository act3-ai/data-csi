#!/usr/bin/env bash

export ver=$1
echo "$ver" >VERSION

yq e '(.version = env(ver)) | (.appVersion = env(ver))' -i charts/csi-bottle/Chart.yaml
yq e '.image.tag = "v" + env(ver)' -i charts/csi-bottle/values.yaml

tool=$2

rm docs/cli/*

# Set HOME to the literal "HOMEDIR" so documentation does not contain the user's home directory
HOME=HOMEDIR NO_COLOR=1 "$tool" gendocs md --only-commands docs/cli/

# Clean up go caching stuff that gets placed in the dummy "HOMEDIR" directory
rm -rf HOMEDIR
