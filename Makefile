all: build
.PHONY: all manifests

OUTPUT_DIR 						:= "./_output"
ARTIFACTS 						:= "./artifacts"
KUBE_MANIFESTS_DIR 				:= "$(OUTPUT_DIR)/deployment"
GO=GO111MODULE=on GOFLAGS=-mod=vendor go
GO_BUILD_BINDIR := bin
GO_TEST_PACKAGES :=./pkg/... ./cmd/...

KUBECTL = kubectl
VERSION := 4.13

OPERATOR_NAMESPACE 			:= runoncedurationoverride-operator
OPERATOR_DEPLOYMENT_NAME 	:= runoncedurationoverride-operator

CODEGEN_OUTPUT_PACKAGE :=github.com/openshift/run-once-duration-override-operator/pkg/generated
CODEGEN_API_PACKAGE :=github.com/openshift/run-once-duration-override-operator/pkg/apis
CODEGEN_GROUPS_VERSION :=runoncedurationoverride:v1
CODEGEN_GO_HEADER_FILE := boilerplate.go.txt

export OLD_OPERATOR_IMAGE_URL_IN_CSV 	= quay.io/openshift/runoncedurationoverride-rhel8-operator:$(VERSION)
export OLD_OPERAND_IMAGE_URL_IN_CSV 	= quay.io/openshift/runoncedurationoverride-rhel8:$(VERSION)
export CSV_FILE_PATH_IN_REGISTRY_IMAGE 	= /manifests/stable/runoncedurationoverride-operator.clusterserviceversion.yaml

LOCAL_OPERATOR_IMAGE	?= quay.io/redhat/runoncedurationoverride-operator:latest
LOCAL_OPERAND_IMAGE 	?= quay.io/redhat/runoncedurationoverride:latest
export LOCAL_OPERATOR_IMAGE
export LOCAL_OPERAND_IMAGE
export LOCAL_OPERATOR_REGISTRY_IMAGE

# Include the library makefile
include $(addprefix ./vendor/github.com/openshift/build-machinery-go/make/, \
	golang.mk \
	targets/openshift/images.mk \
	targets/openshift/codegen.mk \
	targets/openshift/deps.mk \
	targets/openshift/crd-schema-gen.mk \
)

# build image for ci
CI_IMAGE_REGISTRY ?=registry.ci.openshift.org
$(call build-image,runoncedurationoverride-operator,$(CI_IMAGE_REGISTRY)/ocp/4.13:run-once-duration-override-operator,./images/ci/Dockerfile,.)

REGISTRY_SETUP_BINARY := bin/registry-setup

$(REGISTRY_SETUP_BINARY): GO_BUILD_PACKAGES =./test/registry-setup/...
$(REGISTRY_SETUP_BINARY): build

# build image for dev use.
dev-image:
	docker build -t $(DEV_IMAGE_REGISTRY):$(IMAGE_TAG) -f ./images/dev/Dockerfile.dev .

dev-push:
	docker push $(DEV_IMAGE_REGISTRY):$(IMAGE_TAG)

.PHONY: vendor
vendor:
	go mod vendor
	go mod tidy

clean:
	rm -rf $(OUTPUT_DIR)

# oc binary should be in test pod's /tmp/shared dir
e2e-ci: DEPLOY_MODE := ci
e2e-ci: KUBECTL=$(shell which oc)
e2e-ci: deploy e2e

e2e-local: DEPLOY_MODE=local
e2e-local: deploy e2e

operator-registry-deploy-local: operator-registry-generate operator-registry-image-ci operator-registry-deploy
operator-registry-deploy-ci: operator-registry-generate operator-registry-deploy

# TODO: Use alpha-build-machinery for codegen
PKG=github.com/openshift/run-once-duration-override-operator
CODEGEN_INTERNAL:=./vendor/k8s.io/code-generator/generate-internal-groups.sh

codegen:
	docker build -t cro:codegen -f Dockerfile.codegen .
	docker run --name cro-codegen cro:codegen /bin/true
	docker cp cro-codegen:/go/src/github.com/openshift/run-once-duration-override-operator/pkg/generated/. ./pkg/generated
	docker cp cro-codegen:/go/src/github.com/openshift/run-once-duration-override-operator/pkg/apis/. ./pkg/apis
	docker rm cro-codegen

codegen-internal: export GO111MODULE := off
codegen-internal:
	mkdir -p vendor/k8s.io/code-generator/hack
	cp boilerplate.go.txt vendor/k8s.io/code-generator/hack/boilerplate.go.txt
	$(CODEGEN_INTERNAL) deepcopy,conversion,client,lister,informer $(PKG)/pkg/generated $(PKG)/pkg/apis $(PKG)/pkg/apis "apps:v1"

# deploy the operator using kube manifests (no OLM)
deploy: KUBE_MANIFESTS_SOURCE := "$(ARTIFACTS)/deploy"
deploy: DEPLOYMENT_YAML := "$(KUBE_MANIFESTS_DIR)/300_deployment.yaml"
deploy: CONFIGMAP_ENV_FILE := "$(KUBE_MANIFESTS_DIR)/registry-env.yaml"
deploy: $(REGISTRY_SETUP_BINARY)
deploy:
	rm -rf $(KUBE_MANIFESTS_DIR)
	mkdir -p $(KUBE_MANIFESTS_DIR)
	cp -r $(KUBE_MANIFESTS_SOURCE)/* $(KUBE_MANIFESTS_DIR)/
	cp manifests/stable/runoncedurationoverride.crd.yaml $(KUBE_MANIFESTS_DIR)/
	cp $(ARTIFACTS)/registry-env.yaml $(KUBE_MANIFESTS_DIR)/

	$(REGISTRY_SETUP_BINARY) --mode=$(DEPLOY_MODE) --olm=false --configmap=$(CONFIGMAP_ENV_FILE)
	./hack/update-image-url.sh "$(CONFIGMAP_ENV_FILE)" "$(DEPLOYMENT_YAML)"

	$(KUBECTL) apply -n $(OPERATOR_NAMESPACE) -f $(KUBE_MANIFESTS_DIR)

# run e2e test(s)
e2e:
	$(KUBECTL) -n $(OPERATOR_NAMESPACE) rollout status -w deployment/runoncedurationoverride-operator
	export GO111MODULE=on
	$(GO) test -v -count=1 -timeout=15m ./test/e2e/... --kubeconfig=${KUBECONFIG} --namespace=$(OPERATOR_NAMESPACE)

.PHONY: build-testutil
build-testutil: bin/yaml2json bin/json2yaml ## Build utilities needed by tests

# utilities needed by tests
bin/yaml2json: cmd/testutil/yaml2json/yaml2json.go
	mkdir -p bin
	go build $(GOGCFLAGS) -ldflags "$(LD_FLAGS)" -o bin/ "$(PKG)/cmd/testutil/yaml2json"
bin/json2yaml: cmd/testutil/json2yaml/json2yaml.go
	mkdir -p bin
	go build $(GOGCFLAGS) -ldflags "$(LD_FLAGS)" -o bin/ "$(PKG)/cmd/testutil/json2yaml"

regen-crd:
	go build -o _output/tools/bin/controller-gen ./vendor/sigs.k8s.io/controller-tools/cmd/controller-gen
	cp manifests/stable/runoncedurationoverride.crd.yaml manifests/stable/operator.openshift.io_runoncedurationoverrides.yaml
	./_output/tools/bin/controller-gen crd paths=./pkg/apis/runoncedurationoverride/v1/... schemapatch:manifests=./manifests/stable
	mv manifests/stable/operator.openshift.io_runoncedurationoverrides.yaml manifests/stable/runoncedurationoverride.crd.yaml
