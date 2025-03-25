FROM brew.registry.redhat.io/rh-osbs/openshift-golang-builder:rhel_9_1.22 as builder
WORKDIR /go/src/github.com/openshift/run-once-duration-override-operator
COPY . .

RUN mkdir licenses
COPY ./LICENSE licenses/.

ARG OPERATOR_IMAGE=registry.stage.redhat.io/run-once-duration-override-operator/run-once-duration-override-rhel9-operator@sha256:b6b6003f7e3b42c55745af875715d9c5a45496cbd6b312925d1c1956b1ce9b13
ARG OPERAND_IMAGE=registry.stage.redhat.io/run-once-duration-override-operator/run-once-duration-override-rhel9@sha256:fc6d1b76912cc16daa1079c2e7e95a0bd923e9537ebfcd9c16b49be14bcf4cc6
ARG REPLACED_OPERATOR_IMG=registry-proxy.engineering.redhat.com/rh-osbs/run-once-duration-override-rhel9-operator:latest
ARG REPLACED_OPERAND_IMG=registry-proxy.engineering.redhat.com/rh-osbs/run-once-duration-override-rhel-9:latest

RUN hack/replace-image.sh manifests ${REPLACED_OPERATOR_IMG} ${OPERATOR_IMAGE}
RUN hack/replace-image.sh manifests ${REPLACED_OPERAND_IMG} ${OPERAND_IMAGE}

FROM registry.redhat.io/rhel9-4-els/rhel-minimal:9.4

COPY --from=builder /go/src/github.com/openshift/run-once-duration-override-operator/manifests /manifests
COPY --from=builder /go/src/github.com/openshift/run-once-duration-override-operator/metadata /metadata
COPY --from=builder /go/src/github.com/openshift/run-once-duration-override-operator/licenses /licenses

LABEL operators.operatorframework.io.bundle.mediatype.v1="registry+v1"
LABEL operators.operatorframework.io.bundle.manifests.v1=manifests/
LABEL operators.operatorframework.io.bundle.metadata.v1=metadata/
LABEL operators.operatorframework.io.bundle.package.v1="run-once-duration-override-operator"
LABEL operators.operatorframework.io.bundle.channels.v1=stable
LABEL operators.operatorframework.io.bundle.channel.default.v1=stable
LABEL operators.operatorframework.io.metrics.builder=operator-sdk-v1.34.2
LABEL operators.operatorframework.io.metrics.mediatype.v1=metrics+v1
LABEL operators.operatorframework.io.metrics.project_layout=go.kubebuilder.io/v4

LABEL com.redhat.component="run-once-duration-override-operator-bundle-container"
LABEL description="Run Once Duration Override mutating admission webhook support for OpenShift based on RHEL 9"
LABEL distribution-scope="public"
LABEL name="run-once-duration-override-operator-metadata-rhel-9"
LABEL release="1.2.1"
LABEL version="1.2.1"
LABEL url="https://github.com/openshift/run-once-duration-override-operator"
LABEL vendor="Red Hat, Inc."
LABEL summary="Run Once Duration Override mutating admission webhook support for OpenShift"
LABEL io.openshift.expose-services=""
LABEL io.k8s.display-name="run-once-duration-override-operator based on RHEL 9"
LABEL io.k8s.description="Run Once Duration Override mutating admission webhook support for OpenShift based on RHEL 9"
LABEL io.openshift.tags="openshift,run-once-duration-override-operator"
LABEL com.redhat.delivery.operator.bundle=true
LABEL com.redhat.openshift.versions="v4.17"
LABEL com.redhat.delivery.appregistry=true
LABEL maintainer="AOS workloads team, <aos-workloads-staff@redhat.com>"

USER 1001
