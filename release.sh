#!/usr/bin/env bash

export ver=$1
echo "$ver" >VERSION

tool=$2

rm docs/cli/*

# Set HOME to the literal "HOMEDIR" so documentation does not contain the user's home directory
HOME=HOMEDIR NO_COLOR=1 "$tool" gendocs md --only-commands docs/cli/

# Clean up go caching stuff that gets placed in the dummy "HOMEDIR" directory
rm -rf HOMEDIR
