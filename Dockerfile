FROM brew.registry.redhat.io/rh-osbs/openshift-golang-builder:rhel_9_1.24 as builder
WORKDIR /go/src/github.com/openshift/run-once-duration-override-operator
COPY . .
RUN make build --warn-undefined-variables

FROM registry.access.redhat.com/ubi9/ubi-minimal:latest@sha256:90bd85dcd061d1ad6dbda70a867c41958c04a86462d05c631f8205e8870f28f8
COPY --from=builder /go/src/github.com/openshift/run-once-duration-override-operator/run-once-duration-override-operator /usr/bin/
RUN mkdir /licenses
COPY --from=builder /go/src/github.com/openshift/run-once-duration-override-operator/LICENSE /licenses/.

LABEL io.k8s.display-name="Run Once Duration Override Operator based on RHEL 9" \
      io.k8s.description="This is a component of OpenShift and manages the Run Once Duration Override mutating admission webhook based on RHEL 9" \
      distribution-scope="public" \
      com.redhat.component="run-once-duration-override-operator-container" \
      name="run-once-duration-override-operator/run-once-duration-override-rhel9-operator" \
      cpe="cpe:/a:redhat:run_once_duration_override_operator:1.4::el9" \
      release="1.4.0" \
      version="1.4.0" \
      url="https://github.com/openshift/run-once-duration-override-operator" \
      vendor="Red Hat, Inc." \
      summary="run-once-duration-override-operator" \
      io.openshift.expose-services="" \
      io.openshift.tags="openshift,run-once-duration-override-operator" \
      description="run-once-duration-override-operator-container" \
      maintainer="AOS workloads team, <aos-workloads-staff@redhat.com>"

USER nobody
