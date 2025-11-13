FROM brew.registry.redhat.io/rh-osbs/openshift-golang-builder:rhel_9_1.24 as builder
WORKDIR /go/src/github.com/openshift/run-once-duration-override-operator
COPY . .
RUN make build --warn-undefined-variables

FROM registry.access.redhat.com/ubi9/ubi-minimal:latest@sha256:53ea1f6d835898acda5becdb3f8b1292038a480384bbcf994fc0bcf1f7e8eaf7
COPY --from=builder /go/src/github.com/openshift/run-once-duration-override-operator/run-once-duration-override-operator /usr/bin/
RUN mkdir /licenses
COPY --from=builder /go/src/github.com/openshift/run-once-duration-override-operator/LICENSE /licenses/.

LABEL io.k8s.display-name="Run Once Duration Override Operator based on RHEL 9" \
      io.k8s.description="This is a component of OpenShift and manages the Run Once Duration Override mutating admission webhook based on RHEL 9" \
      com.redhat.component="run-once-duration-override-operator-container" \
      name="run-once-duration-override-operator/run-once-duration-override-rhel9-operator" \
      cpe="cpe:/a:redhat:run_once_duration_override_operator:1.3::el9" \
      summary="run-once-duration-override-operator" \
      io.openshift.expose-services="" \
      io.openshift.tags="openshift,run-once-duration-override-operator" \
      description="run-once-duration-override-operator-container" \
      maintainer="AOS workloads team, <aos-workloads-staff@redhat.com>"

USER nobody
