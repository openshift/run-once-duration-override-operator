FROM registry.redhat.io/openshift/golang-builder:golang-builder-v1.26-rhel9 AS builder
WORKDIR /go/src/github.com/openshift/run-once-duration-override-operator
COPY . .
RUN make build --warn-undefined-variables

FROM registry.access.redhat.com/ubi9/ubi-minimal:latest@sha256:08d4c70f1baf6537a0728656cf7942d561e74a35916509d205b6519861266352
COPY --from=builder /go/src/github.com/openshift/run-once-duration-override-operator/run-once-duration-override-operator /usr/bin/
RUN mkdir /licenses
COPY --from=builder /go/src/github.com/openshift/run-once-duration-override-operator/LICENSE /licenses/.

LABEL io.k8s.display-name="Run Once Duration Override Operator based on RHEL 9" \
      io.k8s.description="This is a component of OpenShift and manages the Run Once Duration Override mutating admission webhook based on RHEL 9" \
      distribution-scope="public" \
      com.redhat.component="run-once-duration-override-operator-container" \
      name="run-once-duration-override-operator/run-once-duration-override-rhel9-operator" \
      cpe="cpe:/a:redhat:run_once_duration_override_operator:1.5::el9" \
      release="1.5.0" \
      version="1.5.0" \
      url="https://github.com/openshift/run-once-duration-override-operator" \
      vendor="Red Hat, Inc." \
      summary="run-once-duration-override-operator" \
      io.openshift.expose-services="" \
      io.openshift.tags="openshift,run-once-duration-override-operator" \
      description="run-once-duration-override-operator-container" \
      maintainer="AOS workloads team, <aos-workloads-staff@redhat.com>"

USER nobody
