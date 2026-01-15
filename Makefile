all: build
.PHONY: all manifests




# Include the library makefile
include $(addprefix ./vendor/github.com/openshift/build-machinery-go/make/, \
	golang.mk \
	targets/openshift/images.mk \
	targets/openshift/codegen.mk \
	targets/openshift/deps.mk \
	targets/openshift/crd-schema-gen.mk \
)

# Exclude e2e tests from unit testing
GO_TEST_PACKAGES :=./pkg/... ./cmd/...
GO_BUILD_FLAGS :=-tags strictfipsruntime

CI_IMAGE_REGISTRY ?=registry.ci.openshift.org

CODEGEN_OUTPUT_PACKAGE :=github.com/openshift/run-once-duration-override-operator/pkg/generated
CODEGEN_API_PACKAGE :=github.com/openshift/run-once-duration-override-operator/pkg/apis
CODEGEN_GROUPS_VERSION :=runoncedurationoverride:v1
CODEGEN_GO_HEADER_FILE := boilerplate.go.txt

# build image for ci
$(call build-image,runoncedurationoverride-operator,$(CI_IMAGE_REGISTRY)/ocp/4.14:run-once-duration-override-operator,./images/ci/Dockerfile,.)

$(call verify-golang-versions,Dockerfile.rhel7)

test-e2e: GO_TEST_PACKAGES :=./test/e2e
# the e2e imports pkg/cmd which has a data race in the transport library with the library-go init code
test-e2e: GO_TEST_FLAGS :=-v -timeout=3h
test-e2e: test-unit
.PHONY: test-e2e

regen-crd:
	go build -o _output/tools/bin/controller-gen ./vendor/sigs.k8s.io/controller-tools/cmd/controller-gen
	cp manifests/runoncedurationoverride.crd.yaml manifests/operator.openshift.io_runoncedurationoverrides.yaml
	./_output/tools/bin/controller-gen crd paths=./pkg/apis/runoncedurationoverride/v1/... schemapatch:manifests=./manifests output:crd:dir=./manifests
	mv manifests/operator.openshift.io_runoncedurationoverrides.yaml manifests/runoncedurationoverride.crd.yaml
	# Remove leading --- from CRD file
	sed -i '1{/^---$$/d;}' manifests/runoncedurationoverride.crd.yaml
	# Remove .annotations to drop controller-gen.kubebuilder.io/version as the only key set
	yq eval 'del(.metadata.annotations)' -i manifests/runoncedurationoverride.crd.yaml
	cp manifests/runoncedurationoverride.crd.yaml test/e2e/bindata/assets/08_crd.yaml
	cp manifests/runoncedurationoverride.crd.yaml deploy/02_runoncedurationoverride.crd.yaml

generate: update-codegen-crds generate-clients
.PHONY: generate

generate-clients:
	bash ./vendor/k8s.io/code-generator/generate-groups.sh all github.com/openshift/run-once-duration-override-operator/pkg/generated github.com/openshift/run-once-duration-override-operator/pkg/apis runoncedurationoverride:v1
.PHONY: generate-clients

clean:
	$(RM) -rf ./run-once-duration-override-operator
	$(RM) -rf ./_output
.PHONY: clean
