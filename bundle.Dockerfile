FROM brew.registry.redhat.io/rh-osbs/openshift-golang-builder:rhel_9_1.24 as builder
WORKDIR /go/src/github.com/openshift/run-once-duration-override-operator
COPY . .

RUN mkdir licenses
COPY ./LICENSE licenses/.

ARG OPERATOR_IMAGE=registry.redhat.io/run-once-duration-override-operator/run-once-duration-override-rhel9-operator@sha256:386b07ea8e48deff6af9c5555f3a7686d99334fce523f4cf67ba6dc2e251c2db
# artificial distance to avoid rebase conflicts when the operand and the operator image gets updated at the same time
#
#
#
#
#
#
#
ARG OPERAND_IMAGE=registry.redhat.io/run-once-duration-override-operator/run-once-duration-override-rhel9@sha256:005e2205a7bfa24c8582b23cc7dd19f6de7c1ff7a5f9c0c6b4736abfde71116e
ARG REPLACED_OPERATOR_IMG=registry-proxy.engineering.redhat.com/rh-osbs/run-once-duration-override-rhel9-operator:latest
ARG REPLACED_OPERAND_IMG=registry-proxy.engineering.redhat.com/rh-osbs/run-once-duration-override-rhel-9:latest

RUN hack/replace-image.sh manifests ${REPLACED_OPERATOR_IMG} ${OPERATOR_IMAGE}
RUN hack/replace-image.sh manifests ${REPLACED_OPERAND_IMG} ${OPERAND_IMAGE}

FROM registry.access.redhat.com/ubi9/ubi-minimal:latest@sha256:c5478a52c410e71c53839923c83a1480199a1e74ce5736fe3e3a5578dc399102

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
LABEL cpe="cpe:/a:redhat:run_once_duration_override_operator:1.4::el9"
LABEL release="1.4.0"
LABEL version="1.4.0"
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
