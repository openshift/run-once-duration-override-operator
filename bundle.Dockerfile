FROM brew.registry.redhat.io/rh-osbs/openshift-golang-builder:rhel_9_1.23 as builder
WORKDIR /go/src/github.com/openshift/run-once-duration-override-operator
COPY . .

RUN mkdir licenses
COPY ./LICENSE licenses/.

ARG OPERATOR_IMAGE=registry.redhat.io/run-once-duration-override-operator/run-once-duration-override-rhel9-operator@sha256:c57c6c47835b15dbc865478e8c3accf0b8a6509c3a3a2c7335c543efdbc62d97
ARG OPERAND_IMAGE=registry.redhat.io/run-once-duration-override-operator/run-once-duration-override-rhel9@sha256:e6698c86942b82a6b61cf4be5e74dbea58534d0b367ba37d0ed62c2aac453b67
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
LABEL name="run-once-duration-override-operator/run-once-duration-override-operator-bundle"
LABEL cpe="cpe:/a:redhat:run_once_duration_override_operator:1.3::el9"
LABEL release="1.3.0"
LABEL version="1.3.0"
LABEL url="https://github.com/openshift/run-once-duration-override-operator"
LABEL vendor="Red Hat, Inc."
LABEL summary="Run Once Duration Override mutating admission webhook support for OpenShift"
LABEL io.openshift.expose-services=""
LABEL io.k8s.display-name="run-once-duration-override-operator based on RHEL 9"
LABEL io.k8s.description="Run Once Duration Override mutating admission webhook support for OpenShift based on RHEL 9"
LABEL io.openshift.tags="openshift,run-once-duration-override-operator"
LABEL com.redhat.delivery.operator.bundle=true
LABEL com.redhat.openshift.versions="v4.19"
LABEL com.redhat.delivery.appregistry=true
LABEL maintainer="AOS workloads team, <aos-workloads-staff@redhat.com>"

USER 1001
