#!/bin/sh -e

vagrant ssh -c "sudo ./csi-bottle checkpath $1"
