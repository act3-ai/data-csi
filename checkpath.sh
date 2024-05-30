#!/bin/sh -e

vagrant ssh -c "sudo /vagrant/bin/csi-bottle-linux-amd64 checkpath $1"
