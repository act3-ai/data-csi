# Copyright 2019 The Kubernetes Authors.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

all: build

# Revision that gets built into each binary via the main.version
# string. Uses the `git describe` output based on the most recent
# version tag with a short revision suffix or, if nothing has been
# tagged yet, just the revision.
#
# Beware that tags may also be missing in shallow clones as done by
# some CI systems (like TravisCI, which pulls only 50 commits).
REV=$(shell git describe --long --tags --match='v*' --dirty 2>/dev/null || git rev-list -n1 HEAD)

############################################################
# External tools
############################################################

# renovate: datasource=go depName=github.com/rexray/gocsi
CSC_VERSION?=v1.2.2

# renovate: datasource=go depName=github.com/kubernetes-csi/csi-test
CSI_SANITY_VERSION?=v5.2.0

tool/csc: tool/.csc.$(CSC_VERSION)
	GOBIN=$(PWD)/tool go install github.com/rexray/gocsi/csc@$(CSC_VERSION)

tool/.csc.$(CSC_VERSION):
	@rm -f tool/.csc.*
	@mkdir -p tool
	touch $@

tool/csi-sanity: tool/.csi-sanity.$(CSI_SANITY_VERSION)
	GOBIN=$(PWD)/tool go install github.com/kubernetes-csi/csi-test/v5/cmd/csi-sanity@$(CSI_SANITY_VERSION)

tool/.csi-sanity.$(CSI_SANITY_VERSION):
	@rm -f tool/.csi-sanity.*
	@mkdir -p tool
	touch $@


.PHONY: tool
tool: tool/csc tool/csi-sanity

CMD=sudo ./csi-bottle serve --endpoint tcp://0.0.0.0:10000 --nodeid CSINode -v=4
FULL_CMD=$(CMD) $(EXTRA_ARGS)

# Run first: dagger call with-netrc --netrc=file:~/.netrc build --platform=linux/amd64 export --path=./bin/csi-bottle-linux-amd64-test
.PHONY: vagrant
vagrant: build
	vagrant up --provider libvirt
	vagrant ssh -c '$(FULL_CMD)'

.PHONY: vagrant-tree
vagrant-tree:
	vagrant ssh -c 'sudo tree -ha /tmp/csi'

