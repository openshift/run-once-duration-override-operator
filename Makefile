all: build
.PHONY: all manifests




# Include the library makefile
# NOTE: codegen.mk and crd-schema-gen.mk are not included: they drive the
# removed generate-groups.sh; the codegen targets are defined locally below.
include $(addprefix ./vendor/github.com/openshift/build-machinery-go/make/, \
	golang.mk \
	targets/openshift/images.mk \
	targets/openshift/deps.mk \
)

# Exclude e2e tests from unit testing
GO_TEST_PACKAGES :=./pkg/... ./cmd/...
GO_BUILD_FLAGS :=-tags strictfipsruntime

CI_IMAGE_REGISTRY ?=registry.ci.openshift.org

# build image for ci
$(call build-image,runoncedurationoverride-operator,$(CI_IMAGE_REGISTRY)/ocp/4.14:run-once-duration-override-operator,./images/ci/Dockerfile,.)

$(call verify-golang-versions,Dockerfile.rhel7)

test-e2e: GO_TEST_PACKAGES :=./test/e2e
# the e2e imports pkg/cmd which has a data race in the transport library with the library-go init code
test-e2e: GO_TEST_FLAGS :=-v -timeout=3h
test-e2e: test-unit
.PHONY: test-e2e

# Code generation: clients via kube_codegen.sh (hack/update-codegen.sh),
# CRDs via the vendored controller-gen.
CONTROLLER_GEN :=./_output/tools/bin/controller-gen
CONTROLLER_GEN_SRC :=./vendor/sigs.k8s.io/controller-tools/cmd/controller-gen

$(CONTROLLER_GEN): $(CONTROLLER_GEN_SRC)
	$(GO) build $(GO_MOD_FLAGS) -o $(CONTROLLER_GEN) $(CONTROLLER_GEN_SRC)

generate: update-codegen-crds update-codegen
.PHONY: generate

update-codegen:
	GO=GO111MODULE=on GOFLAGS=-mod=readonly hack/update-codegen.sh
.PHONY: update-codegen

# backwards compatible alias
generate-clients: update-codegen
.PHONY: generate-clients

update-codegen-crds: $(CONTROLLER_GEN)
	cp manifests/runoncedurationoverride.crd.yaml manifests/operator.openshift.io_runoncedurationoverrides.yaml
	# the crd generator writes <group>_<plural>.yaml, which mv applies below
	# (schemapatch is not run: its output was always discarded)
	$(CONTROLLER_GEN) crd paths=./pkg/apis/runoncedurationoverride/v1/... output:crd:dir=./manifests
	mv manifests/operator.openshift.io_runoncedurationoverrides.yaml manifests/runoncedurationoverride.crd.yaml
	# Remove leading --- from CRD file
	sed -i '1{/^---$$/d;}' manifests/runoncedurationoverride.crd.yaml
	# Remove .annotations to drop controller-gen.kubebuilder.io/version as the only key set
	yq eval 'del(.metadata.annotations)' -i manifests/runoncedurationoverride.crd.yaml
	cp manifests/runoncedurationoverride.crd.yaml test/e2e/bindata/assets/08_crd.yaml
	cp manifests/runoncedurationoverride.crd.yaml deploy/02_runoncedurationoverride.crd.yaml
.PHONY: update-codegen-crds

# backwards compatible alias
regen-crd: update-codegen-crds
.PHONY: regen-crd

verify-codegen: update-codegen
	git diff --exit-code -- pkg/apis pkg/generated
.PHONY: verify-codegen

verify-codegen-crds: update-codegen-crds
	git diff --exit-code -- manifests deploy test/e2e/bindata
.PHONY: verify-codegen-crds

verify-generated: verify-codegen verify-codegen-crds
.PHONY: verify-generated

verify: verify-generated
.PHONY: verify

update-generated: update-codegen update-codegen-crds
.PHONY: update-generated

update: update-generated
.PHONY: update

clean:
	$(RM) -rf ./run-once-duration-override-operator
	$(RM) -rf ./_output
.PHONY: clean
