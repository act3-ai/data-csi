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

# This is the default. It can be overridden in the main Makefile after
# including build.make.
REGISTRY_NAME=zot.lion.act3-ace.ai
IMAGE_REPO ?= $(REGISTRY_NAME)/ace/data/csi

.PHONY: generate
generate: tool/controller-gen tool/crd-ref-docs
	go generate ./...

.PHONY: build
build: generate
	@mkdir -p bin
	go build -o bin/csi-bottle ./cmd/csi-bottle

.PHONY: build
build-linux: generate
	@mkdir -p bin
	GOOS=linux GOARCH=amd64 go build -o bin/csi-bottle-linux-amd64 ./cmd/csi-bottle

.PHONY: install
install: generate
	go install ./cmd/csi-bottle

.PHONY: build-docker
build-docker: generate
	docker build -t reg.git.act3-ace.com/ace/data/csi .

.PHONY: docker-run
docker-run: generate
	docker run -it reg.git.act3-ace.com/ace/data/csi:latest

.PHONY: ko
ko: tool/ko
	VERSION=$(REV) KO_DOCKER_REPO=$(IMAGE_REPO) tool/ko build -B --platform=all --image-label version=$(REV) ./cmd/csi-bottle

.PHONY: test
test: generate
	go test ./...

.PHONY: lint
lint: tool/golangci-lint
	tool/golangci-lint run

############################################################
# External tools
############################################################

# renovate: datasource=go depName=sigs.k8s.io/controller-tools
CONTROLLER_GEN_VERSION?=v0.15.0
# renovate: datasource=go depName=github.com/elastic/crd-ref-docs
CRD_REF_DOCS_VERSION?=v0.1.0
# renovate: datasource=go depName=github.com/google/ko
KO_VERSION?=v0.16.0
# renovate: datasource=go depName=github.com/golangci/golangci-lint
GOLANGCILINT_VERSION?=v1.58.2

# renovate: datasource=go depName=github.com/rexray/gocsi
CSC_VERSION?=v1.2.2

# renovate: datasource=go depName=github.com/kubernetes-csi/csi-test
CSI_SANITY_VERSION?=v5.2.0

# Installs all tools
.PHONY: tool
tool: tool/controller-gen tool/crd-ref-docs tool/ko tool/golangci-lint

# controller-gen: generates copy functions for CRDs
tool/controller-gen: tool/.controller-gen.$(CONTROLLER_GEN_VERSION)
	GOBIN=$(PWD)/tool go install sigs.k8s.io/controller-tools/cmd/controller-gen@$(CONTROLLER_GEN_VERSION)

tool/.controller-gen.$(CONTROLLER_GEN_VERSION):
	@rm -f tool/.controller-gen.*
	@mkdir -p tool
	touch $@

# crd-ref-docs: Generates markdown documentation for CRDs
tool/crd-ref-docs: tool/.crd-ref-docs.$(CRD_REF_DOCS_VERSION)
	GOBIN=$(PWD)/tool go install github.com/elastic/crd-ref-docs@$(CRD_REF_DOCS_VERSION)

tool/.crd-ref-docs.$(CRD_REF_DOCS_VERSION):
	@rm -f tool/.crd-ref-docs.*
	@mkdir -p tool
	touch $@

# ko: builds application images for Go projects
tool/ko: tool/.ko.$(KO_VERSION)
	GOBIN=$(PWD)/tool go install github.com/google/ko@$(KO_VERSION)

tool/.ko.$(KO_VERSION):
	@rm -f tool/.ko.*
	@mkdir -p tool
	touch $@

# golangci-lint: lints Go code
tool/golangci-lint: tool/.golangci-lint.$(GOLANGCILINT_VERSION)
	GOBIN=$(PWD)/tool go install github.com/golangci/golangci-lint/cmd/golangci-lint@$(GOLANGCILINT_VERSION)

tool/.golangci-lint.$(GOLANGCILINT_VERSION):
	@rm -f tool/.golangci-lint.*
	@mkdir -p tool
	touch $@


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
tool: tool/ko tool/golangci-lint tool/csc tool/csi-sanity


CMD=sudo /vagrant/bin/csi-bottle-linux-amd64 serve --endpoint tcp://0.0.0.0:10000 --nodeid CSINode -v=4
FULL_CMD=$(CMD) $(EXTRA_ARGS)

.PHONY: vagrant
vagrant: build-linux
	vagrant up
	vagrant ssh -c '$(FULL_CMD)'

.PHONY: vagrant-tree
vagrant-tree:
	vagrant ssh -c 'sudo tree -ha /tmp/csi'

