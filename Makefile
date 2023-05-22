PROJECT_NAME=greatdb-operator
COMMIT_ID=$(shell git rev-parse --short HEAD)
COMMIT_SUM=$(shell git rev-list --all --count)
TAG=$(shell git describe --abbrev=0 --tags)
DBINIT_TAG=$(shell cd dbinit && git describe --abbrev=0 --tags)
MACHINE=$(shell uname -m)

RELEASE_DATE=$(shell git log -1 --format=%ad --date=format:'%Y.%m.%d' ${TAG})

IMG=docker.greatrds.com:5000/${PROJECT_NAME}:$(TAG)-$(COMMIT_ID)
# ENVTEST_K8S_VERSION refers to the version of kubebuilder assets to be downloaded by envtest binary.
ENVTEST_K8S_VERSION = 1.22

# Get the currently used golang install path (in GOPATH/bin, unless GOBIN is set)
ifeq (,$(shell go env GOBIN))
GOBIN=$(shell go env GOPATH)/bin
else
GOBIN=$(shell go env GOBIN)
endif

# Setting SHELL to bash allows bash commands to be executed by recipes.
# This is a requirement for 'setup-envtest.sh' in the test target.
# Options are set to exit when a recipe line exits non-zero or a piped command fails.
SHELL = /usr/bin/env bash -o pipefail
.SHELLFLAGS = -ec

.PHONY: all
all: build

##@ General

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

.PHONY: help
help: ## Display this help.
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

##@ Development
update_codegen:   ## 更新自动生成的client代码
	bash ./hack/update_codegen.sh "greatdb-router-operator" "greatdb:v1alpha1"

.PHONY: manifests
manifests: controller-gen ## Generate WebhookConfiguration, ClusterRole and CustomResourceDefinition objects.
	$(CONTROLLER_GEN) rbac:roleName=greatdb-router-operator-role crd webhook paths="./pkg/apis/greatdb/v1alpha1/..." output:crd:dir=deploy/crd/bases output:rbac:dir=deploy/rbac

.PHONY: generate
generate: controller-gen ## Generate code containing DeepCopy, DeepCopyInto, and DeepCopyObject method implementations.
	$(CONTROLLER_GEN) object:headerFile="hack/boilerplate.go.txt" paths="./pkg/apis/greatdb/v1alpha1/..."

.PHONY: fmt
fmt: ## Run go fmt against code.
	go fmt ./...

.PHONY: vet
vet: ## Run go vet against code.
	go vet ./...

.PHONY: golangci-lint
golangci-lint: ## Run golangci-lint run ./...
	golangci-lint run ./...

.PHONY: test
test: manifests generate fmt vet envtest golangci-lint ## Run tests.
	KUBEBUILDER_ASSETS="$(shell $(ENVTEST) use $(ENVTEST_K8S_VERSION) -p path)" go test ./... -coverprofile cover.out

##@ Build
.PHONY: build-agent
build-agent: ## Build manager binary agent.
	GOOS=linux GOARCH=amd64 go build -o bin/greatdb-agent ./cmd/greatdb-agent/main.go

.PHONY: build
build: generate fmt vet ## Build manager binary.
	GOOS=linux GOARCH=amd64 go build -o bin/server ./cmd/controller-manager/server.go

.PHONY: run
run: manifests generate fmt vet ## Run a controller from your host.
	go run ./cmd/controller-manager/server.go

.PHONY: docker-build
docker-build: ## Build docker image with the manager.
	docker build --network=host --build-arg VERSION="${TAG}-${COMMIT_ID}" --build-arg RELEASE_DATE="${RELEASE_DATE}" -t ${IMG} -f ./Dockerfile .

.PHONY: docker-push
docker-push: ## Push docker image with the manager.
	docker push ${IMG}

.PHONY: docker-save
docker-save: ## Push docker image with the manager.
	docker save -o ${PROJECT_NAME}-${TAG}.tar ${IMG}

.PHONY: docker-load
docker-load: ## Push docker image with the manager.
	docker load -i ${IMG}

##@ Deployment

ifndef ignore-not-found
  ignore-not-found = false
endif

.PHONY: install
install: manifests kustomize ## Install CRDs into the K8s cluster specified in ~/.kube/config.
	$(KUSTOMIZE) build deploy/crd | kubectl apply -f -

.PHONY: uninstall
uninstall: manifests kustomize ## Uninstall CRDs from the K8s cluster specified in ~/.kube/config. Call with ignore-not-found=true to ignore resource not found errors during deletion.
	$(KUSTOMIZE) build deploy/crd | kubectl delete --ignore-not-found=$(ignore-not-found) -f -

.PHONY: deploy
deploy: manifests kustomize ## Deploy controller to the K8s cluster specified in ~/.kube/config.
	cd deploy/manager && $(KUSTOMIZE) edit set image controller=${IMG}
	$(KUSTOMIZE) build deploy/default | kubectl apply -f -

.PHONY: undeploy
undeploy: ## Undeploy controller from the K8s cluster specified in ~/.kube/config. Call with ignore-not-found=true to ignore resource not found errors during deletion.
	$(KUSTOMIZE) build deploy/default | kubectl delete --ignore-not-found=$(ignore-not-found) -f -

CONTROLLER_GEN = $(shell pwd)/bin/controller-gen
.PHONY: controller-gen
controller-gen: ## Download controller-gen locally if necessary.
	test -s $(CONTROLLER_GEN) || GOBIN=$(shell pwd)/bin go install sigs.k8s.io/controller-tools/cmd/controller-gen@v0.9.0

KUSTOMIZE = $(shell pwd)/bin/kustomize
.PHONY: kustomize
kustomize: ## Download kustomize locally if necessary.
	test -s $(KUSTOMIZE) || GOBIN=$(shell pwd)/bin go install sigs.k8s.io/kustomize/kustomize/v4@v4.5.7

ENVTEST = $(shell pwd)/bin/setup-envtest
.PHONY: envtest
envtest: ## Download envtest-setup locally if necessary.
	test -s $(ENVTEST) || GOBIN=$(shell pwd)/bin go install sigs.k8s.io/controller-runtime/tools/setup-envtest@latest

# go-get-tool will 'go get' any package $2 and install it to $1.
PROJECT_DIR := $(shell dirname $(abspath $(lastword $(MAKEFILE_LIST))))
define go-get-tool
@[ -f $(1) ] || { \
set -e ;\
TMP_DIR=$$(mktemp -d) ;\
cd $$TMP_DIR ;\
go mod init tmp ;\
echo "Downloading $(2)" ;\
GOBIN=$(PROJECT_DIR)/bin go get $(2) ;\
rm -rf $$TMP_DIR ;\
}
endef
