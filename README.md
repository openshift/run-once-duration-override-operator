# Overview
This operator manages OpenShift `RunOnceDurationOverride` Admission Webhook Server.

`RunOnceDurationOverride` Admission Webhook Server is located at [run-once-duration-override](https://github.com/openshift/run-once-duration-override).

## Deploy the Operator

### Quick Development

1. Build and push the operator image to a registry:
   ```sh
   export QUAY_USER=${your_quay_user_id}
   export IMAGE_TAG=${your_image_tag}
   podman build -t quay.io/${QUAY_USER}/run-once-duration-override-operator:${IMAGE_TAG} -f Dockerfile.rhel7 .
   podman login quay.io -u ${QUAY_USER}
   podman push quay.io/${QUAY_USER}/run-once-duration-override-operator:${IMAGE_TAG}
   ```

1. Update the image spec under `.spec.template.spec.containers[0].image` field in the `deploy/07_deployment.yaml` Deployment to point to the newly built image

1. Update the `OPERAND_IMAGE` env value under `.spec.template.spec.containers[0].envs` field in the `deploy/07_deployment.yaml` to point to the admission webhook image

1. Update the `.spec.runOnceDurationOverride.spec.activeDeadlineSeconds` under `deploy/08_cr.yaml` as needed

1. Apply the manifests from `deploy` directory:
   ```sh
   oc apply -f deploy/
   ```
