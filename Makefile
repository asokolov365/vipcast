PKG_PREFIX := github.com/asokolov365/vipcast
APP_NAME := vipcast

SHELL := /bin/bash
ROOT := $(shell pwd)

ifeq ($(shell uname -s),Darwin)
TAR := $(shell which gtar)
ifeq (,$(TAR))
	$(error "unable to find gtar (gnu-tar) in $(PATH), run: brew install gnu-tar")
endif
SHA256SUM := shasum -a 256
MAKE_CONCURRENCY ?= $(shell sysctl -n hw.ncpu)
else
TAR := $(shell which tar)
SHA256SUM := sha256sum
MAKE_CONCURRENCY ?= $(shell nproc --all)
endif

MAKEFLAGS += --warn-undefined-variables
MAKE_PARALLEL := $(MAKE) -j $(MAKE_CONCURRENCY)

DATEINFO_TAG ?= $(shell date -u +'%Y%m%d-%H%M%S')
BUILDINFO_TAG ?= $(shell echo $$(git describe --long --all | tr '/' '-')$$( \
	      git diff-index --quiet HEAD -- || echo '-dirty-'$$(git diff-index -u HEAD | openssl sha1 | cut -d' ' -f2 | cut -c 1-8)))

PKG_TAG ?= $(shell git tag -l --points-at HEAD)
ifeq ($(PKG_TAG),)
PKG_TAG := $(BUILDINFO_TAG)
endif

GO_BUILDINFO := -X '$(PKG_PREFIX)/version.Version=$(APP_NAME)-$(DATEINFO_TAG)-$(BUILDINFO_TAG)'

GOLANGCI_LINT_VERSION='v1.51.2'

GOPATH := $(shell go env GOPATH)
GOOS ?= $(shell go env GOOS)
GOARCH ?= $(shell go env GOARCH)

# Docker stuff
DOCKER_IMAGE_LIST := docker image ls --format '{{.Repository}}:{{.Tag}}'
GO_BUILDER_IMAGE := golang:1.21.6-alpine

GITHUB_RELEASE_SPEC_FILE="/tmp/vipcast-github-release"
GITHUB_DEBUG_FILE="/tmp/vipcast-github-debug"

TEST ?= "all"

.PHONY: $(MAKECMDGOALS)

default: all

noop: ;

##@ Build

all: check test test-run ## Command running by default

build-local: ## Create binary for testing locally - ./build/vipcast
	mkdir -p $(ROOT)/build
	# rm needed due to signature caching (https://apple.stackexchange.com/a/428388)
	rm -f "$(ROOT)/build/$(APP_NAME)-dev"
	CGO_ENABLED=0 go build -ldflags "$(GO_BUILDINFO)" -tags "osusergo" -o "$(ROOT)/build/$(APP_NAME)-dev" .

integration: build-docker-local ## run integration tests; filter with `TEST=testname make integration`
	./scripts/test.sh test $(TEST)

build-docker-local: build-linux-amd64-local ## Create docker image for testing locally
	DOCKER_BUILDKIT=1 docker build -t vipcast:dev .

crossbuild: ## Create cross-platform binaries for testing locally
	$(MAKE_PARALLEL) crossbuild-local-all

crossbuild-local-all: \
	build-linux-amd64-local \
	build-linux-arm64-local \
	build-darwin-amd64-local \
	build-darwin-arm64-local \
	build-freebsd-amd64-local \
	build-openbsd-amd64-local

build-linux-amd64-local:
	GOOS=linux GOARCH=amd64 $(MAKE) build-goos-goarch-local

build-linux-arm64-local:
	GOOS=linux GOARCH=arm64 $(MAKE) build-goos-goarch-local

build-darwin-amd64-local:
	GOOS=darwin GOARCH=amd64 $(MAKE) build-goos-goarch-local

build-darwin-arm64-local:
	GOOS=darwin GOARCH=arm64 $(MAKE) build-goos-goarch-local

build-freebsd-amd64-local:
	GOOS=freebsd GOARCH=amd64 $(MAKE) build-goos-goarch-local

build-openbsd-amd64-local:
	GOOS=openbsd GOARCH=amd64 $(MAKE) build-goos-goarch-local

build-goos-goarch-local:
	mkdir -p $(ROOT)/build
	# rm needed due to signature caching (https://apple.stackexchange.com/a/428388)
	rm -f "$(ROOT)/build/$(APP_NAME)-$(GOOS)-$(GOARCH)-dev"
	CGO_ENABLED=0 GOOS=$(GOOS) GOARCH=$(GOARCH) go build -ldflags "$(GO_BUILDINFO)" -tags "osusergo" -o "$(ROOT)/build/$(APP_NAME)-$(GOOS)-$(GOARCH)-dev" .

build-linux-amd64-docker:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 $(MAKE) build-goos-goarch-docker

build-linux-arm64-docker:
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 $(MAKE) build-goos-goarch-docker

build-darwin-amd64-docker:
	CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 $(MAKE) build-goos-goarch-docker

build-darwin-arm64-docker:
	CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 $(MAKE) build-goos-goarch-docker

build-freebsd-amd64-docker:
	CGO_ENABLED=0 GOOS=freebsd GOARCH=amd64 $(MAKE) build-goos-goarch-docker

build-openbsd-amd64-docker:
	CGO_ENABLED=0 GOOS=openbsd GOARCH=amd64 $(MAKE) build-goos-goarch-docker

build-goos-goarch-docker: package-builder
	mkdir -p $(ROOT)/build
	mkdir -p docker-gocache
	docker run --rm $(DOCKER_OPTS) \
		--mount type=bind,src="$(shell pwd)",dst=/vipcast \
		--mount type=bind,src="$(shell pwd)/docker-gocache",dst=/gocache \
		--env GOCACHE=/gocache \
		--workdir /vipcast \
		--env CGO_ENABLED=$(CGO_ENABLED) \
		--env GOOS=$(GOOS) \
		--env GOARCH=$(GOARCH) \
		$(GO_BUILDER_IMAGE) \
		go build -trimpath -buildvcs=false \
			-ldflags "-extldflags '-static' $(GO_BUILDINFO)" \
			-o build/$(APP_NAME)-$(GOOS)-$(GOARCH)-prod .

package-builder: ## Get go-builder docker image
	($(DOCKER_IMAGE_LIST) | grep -q '$(GO_BUILDER_IMAGE)$$') || \
		docker pull $(GO_BUILDER_IMAGE)

##@ Release

release: package-builder ## Build release binaries
	$(MAKE_PARALLEL) release-all

release-all: \
	release-linux-amd64 \
	release-linux-arm64 \
	release-darwin-amd64 \
	release-darwin-arm64 \
	release-freebsd-amd64 \
	release-openbsd-amd64
	
release-linux-amd64:
	GOOS=linux GOARCH=amd64 $(MAKE) release-goos-goarch

release-linux-arm64:
	GOOS=linux GOARCH=arm64 $(MAKE) release-goos-goarch

release-darwin-amd64:
	GOOS=darwin GOARCH=amd64 $(MAKE) release-goos-goarch

release-darwin-arm64:
	GOOS=darwin GOARCH=arm64 $(MAKE) release-goos-goarch

release-freebsd-amd64:
	GOOS=freebsd GOARCH=amd64 $(MAKE) release-goos-goarch

release-openbsd-amd64:
	GOOS=openbsd GOARCH=amd64 $(MAKE) release-goos-goarch

release-goos-goarch: \
	build-$(GOOS)-$(GOARCH)-docker
	cd $(ROOT)/build && \
		$(TAR) --transform="flags=r;s|-$(GOOS)-$(GOARCH)-prod||" \
			--owner=root --group=root \
			-czf $(APP_NAME)-$(GOOS)-$(GOARCH)-$(PKG_TAG).tar.gz \
			$(APP_NAME)-$(GOOS)-$(GOARCH)-prod && \
		$(SHA256SUM) $(APP_NAME)-$(GOOS)-$(GOARCH)-$(PKG_TAG).tar.gz \
			$(APP_NAME)-$(GOOS)-$(GOARCH)-prod | \
			sed s/-$(GOOS)-$(GOARCH)-prod// > $(APP_NAME)-$(GOOS)-$(GOARCH)-$(PKG_TAG)_checksums.txt
	cd $(ROOT)/build && \
		rm -f $(APP_NAME)-$(GOOS)-$(GOARCH)-prod


##@ Clean

clean: ## Remove produced binaries, libraries, and temp files
	@rm -rf $(ROOT)/build/* $(ROOT)/docker-gocache $(ROOT)/tmp
	@rm -f coverage.txt

##@ Checks

check: fmt vet lint govulncheck ## Run formatting, vet, lint, and govulncheck

fmt: gofmt gofumpt

gofmt:
	gofmt -l -w -s $(shell find . -type f -name '*.go'| grep -v "/vendor/")

vet:
	go vet ./...

lint: gci golangci-lint ## Run gci golangci-lint

##@ Testing

test: ## Run go test
	go test ./lib/...

test-race: ## Run go test -v -race
	go test -race -cover ./lib/...

test-full: ## Test cover
	go test -coverprofile=coverage.txt -covermode=atomic ./lib/...
	go tool cover -html=coverage.txt

benchmark: ## Run go test -bench
	go test -bench=. ./...

test-run: build-local
	rm -rf $(ROOT)/tmp
	# YP_SHARD_BASENAME=node YP_REPLICATION_FACTOR=2 $(ROOT)/build/$(APP_NAME)-dev \
	# 	--shards-number=5 \
	# 	--split-at="groups.*.rules" \
	# 	--src="$(ROOT)/testdata/anchors/*.{yml,yaml}" \
	# 	--dst=$(ROOT)/tmp \
	# 	--shard-id=-1

##@ Dependencies

vendor: deps go-mod-tidy go-mod-vendor ## Install Go dependencies

deps:
	go get -u -d ./...

go-mod-tidy:
	go mod tidy -compat=1.20

go-mod-vendor:
	go mod vendor -v

module-versions: ## Print a list of modules which can be updated. Columns are: module current_version date_of_current_version latest_version
	@go list -mod=mod -m -u -f '{{if .Update}} {{printf "%-50v %-40s" .Path .Version}} {{with .Time}} {{ .Format "2006-01-02" -}} {{else}} {{printf "%9s" ""}} {{end}}   {{ .Update.Version}} {{end}}' all


##@ Tools

tools: install-gofumpt install-gci install-golangci-lint install-fieldalignment install-govulncheck install-wwhrd ## Install go dev tools

gofumpt: install-gofumpt
	gofumpt -l -w .

install-gofumpt:
	@which gofumpt || echo "--> Installing gofumpt@latest"; \
		go install mvdan.cc/gofumpt@latest

gci: install-gci
	gci write --skip-vendor .

install-gci:
	@which gci || echo "--> Installing gci@latest"; \
		go install github.com/daixiang0/gci@latest

golangci-lint: install-golangci-lint
	CGO_ENABLED=0 golangci-lint run

install-golangci-lint:
	@which golangci-lint || \
		(echo "--> Installing golangci-lint@$(GOLANGCI_LINT_VERSION)"; \
		curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | \
		sh -s -- -b $(shell go env GOPATH)/bin $(GOLANGCI_LINT_VERSION))

fieldalignment: install-fieldalignment
	fieldalignment -fix ./...

install-fieldalignment:
	@which fieldalignment || echo "--> Installing fieldalignment@latest"; \
		go install golang.org/x/tools/go/analysis/passes/fieldalignment/cmd/fieldalignment@latest

govulncheck: install-govulncheck
	govulncheck ./...

install-govulncheck:
	@which govulncheck || echo "--> Installing govulncheck@latest"; \
		go install golang.org/x/vuln/cmd/govulncheck@latest

##@ Licenses

check-licenses: install-wwhrd ## Checks licenses
	wwhrd check -f .wwhrd.yml

install-wwhrd:
	@which wwhrd || echo "--> Installing wwhrd@latest"; \
		go install github.com/frapposelli/wwhrd@latest

##@ GitHub

github-token-check:
ifndef GITHUB_TOKEN
	$(error missing GITHUB_TOKEN env var. It should be a github token for vipcast project obtained from https://github.com/settings/tokens)
endif

github-tag-check:
ifndef TAG
	$(error missing TAG env var. It must contain github release tag to create)
endif

github-create-release: github-token-check github-tag-check ## Create release draft
	@result=$$(curl -o $(GITHUB_RELEASE_SPEC_FILE) -s -w "%{http_code}" \
		-X POST \
		-H "Accept: application/vnd.github+json" \
		-H "Authorization: token $(GITHUB_TOKEN)" \
		https://api.github.com/repos/asokolov365/vipcast/releases \
		-d '{"tag_name":"$(TAG)","name":"$(TAG)","body":"TODO: put here the changelog for $(TAG) release from CHANGELOG.md","draft":true,"prerelease":false,"generate_release_notes":false}'); \
		if [ $${result} = 201 ]; then \
			release_id=$$(cat $(GITHUB_RELEASE_SPEC_FILE) | grep '"id"' -m 1 | sed -E 's/.* ([[:digit:]]+)\,/\1/'); \
			printf "Created release $(TAG) with id=$${release_id}\n"; \
		else \
			printf "Failed to create release $(TAG)\n"; \
			cat $(GITHUB_RELEASE_SPEC_FILE); \
			exit 1; \
		fi

github-upload-assets: ## Upload assets
	@release_id=$$(cat $(GITHUB_RELEASE_SPEC_FILE) | grep '"id"' -m 1 | sed -E 's/.* ([[:digit:]]+)\,/\1/'); \
	$(foreach file, $(wildcard build/*.tar.gz), FILE=$(file) RELEASE_ID=$${release_id} CONTENT_TYPE="application/x-gzip" $(MAKE) github-upload-asset || exit 1;) \
	$(foreach file, $(wildcard build/*_checksums.txt), FILE=$(file) RELEASE_ID=$${release_id} CONTENT_TYPE="text/plain" $(MAKE) github-upload-asset || exit 1;) 

github-upload-asset: github-token-check
ifndef FILE
	$(error missing FILE env var. It must contain path to file to upload to github release)
endif
	@printf "Uploading $(FILE)\n"
	@result=$$(curl -o $(GITHUB_DEBUG_FILE) -w "%{http_code}" \
		-X POST \
		-H "Accept: application/vnd.github+json" \
		-H "Authorization: token $(GITHUB_TOKEN)" \
		-H "Content-Type: $(CONTENT_TYPE)" \
		--data-binary "@$(FILE)" \
		https://uploads.github.com/repos/asokolov365/vipcast/releases/$(RELEASE_ID)/assets?name=$(notdir $(FILE))); \
		if [ $${result} = 201 ]; then \
			printf "Upload OK: $${result}\n"; \
		elif [ $${result} = 422 ]; then \
			printf "Asset already uploaded, you need to delete it from UI if you want to re-upload it\n"; \
		else \
			printf "Upload failed: $${result}\n"; \
			cat $(GITHUB_DEBUG_FILE); \
			exit 1; \
		fi

github-delete-release: github-token-check ## Delete release
	@release_id=$$(cat $(GITHUB_RELEASE_SPEC_FILE) | grep '"id"' -m 1 | sed -E 's/.* ([[:digit:]]+)\,/\1/'); \
	result=$$(curl -o $(GITHUB_DEBUG_FILE) -s -w "%{http_code}" \
		-X DELETE \
		-H "Accept: application/vnd.github+json" \
		-H "Authorization: token $(GITHUB_TOKEN)" \
		https://api.github.com/repos/asokolov365/vipcast/releases/$${release_id}); \
		if [ $${result} = 204 ]; then \
			printf "Deleted release with id=$${release_id}\n"; \
		else \
			printf "Failed to delete release with id=$${release_id}\n"; \
			cat $(GITHUB_DEBUG_FILE); \
			exit 1; \
		fi

##@ Help

# The help target prints out all targets with their descriptions organized
# beneath their categories. The categories are represented by '##@' and the
# target descriptions by '##'. The awk commands is responsible for reading the
# entire set of makefiles included in this invocation, looking for lines of the
# file as xyz: ## something, and then pretty-format the target and help. Then,
# if there's a line with ##@ something, that gets pretty-printed as a category.
# More info on the usage of ANSI control characters for terminal formatting:
# https://en.wikipedia.org/wiki/ANSI_escape_code#SGR_parameters
# More info on the awk command:
# http://linuxcommand.org/lc3_adv_awk.php
help: ## Display this help.
	@echo -e "\033[32m"
	@echo "Do not use 'make crossbuild' for building binaries for public release!"
	@echo "Before packaging always run 'make clean release'!"
	@echo
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)
