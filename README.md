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

### Building index image from a bundle image (built in Brew)

This process requires access to the Brew building system.

1. List available bundle images (as IMAGE):
  ```
  $ brew list-builds --package=run-once-duration-override-operator-bundle-container
  ```

1. Get pull secret for selected bundle image (as IMAGE_PULL):
  ```
  $ brew --noauth call --json getBuild IMAGE |jq -r '.extra.image.index.pull[0]'
  ```

1. Build the index image (with IMAGE_TAG):
  ```
  $ opm index add --bundles IMAGE_PULL --tag quay.io/${QUAY_USER}/run-once-duration-override-operator-index:IMAGE_TAG
  ```

### OperatorHub install with custom index image

This process refers to building the operator in a way that it can be installed locally via the OperatorHub with a custom index image

 1. Build and push the operator image to a registry:
    ```sh
    export QUAY_USER=${your_quay_user_id}
    export IMAGE_TAG=${your_image_tag}
    podman build -t quay.io/${QUAY_USER}/run-once-duration-override-operator:${IMAGE_TAG} -f Dockerfile.rhel7 .
    podman login quay.io -u ${QUAY_USER}
    podman push quay.io/${QUAY_USER}/run-once-duration-override-operator:${IMAGE_TAG}
    ```

 1. Update the `.spec.install.spec.deployments[0].spec.template.spec.containers[0].image` field in the RODOO CSV under `manifests/runoncedurationoverride-operator.clusterserviceversion.yaml` to point to the newly built image.

 1. Update the `OPERAND_IMAGE` env value under `.spec.install.spec.deployments[0].spec.template.spec.containers[0].envs` field in the RODOO CSV under `manifests/runoncedurationoverride-operator.clusterserviceversion.yaml` to point to the admission webhook image.


 1. build and push the metadata image to a registry (e.g. https://quay.io):
    ```sh
    podman build -t quay.io/${QUAY_USER}/run-once-duration-override-operator-metadata:${IMAGE_TAG} -f Dockerfile.metadata .
    podman push quay.io/${QUAY_USER}/run-once-duration-override-operator-metadata:${IMAGE_TAG}
    ```

 1. build and push image index for operator-registry (pull and build https://github.com/operator-framework/operator-registry/ to get the `opm` binary)
    ```sh
    opm index add --bundles quay.io/${QUAY_USER}/run-once-duration-override-operator-metadata:${IMAGE_TAG} --tag quay.io/${QUAY_USER}/run-once-duration-override-operator-index:${IMAGE_TAG}
    podman push quay.io/${QUAY_USER}/run-once-duration-override-operator-index:${IMAGE_TAG}
    ```

    Don't forget to increase the number of open files, .e.g. `ulimit -n 100000` in case the current limit is insufficient.

 1. create and apply catalogsource manifest (notice to change <<QUAY_USER>> and <<IMAGE_TAG>> to your own values)::
    ```yaml
    apiVersion: operators.coreos.com/v1alpha1
    kind: CatalogSource
    metadata:
      name: run-once-duration-override-operator
      namespace: openshift-marketplace
    spec:
      sourceType: grpc
      image: quay.io/<<QUAY_USER>>/run-once-duration-override-operator-index:<<IMAGE_TAG>>
    ```

 1. create `run-once-duration-override-operator` namespace:
    ```
    $ oc create ns run-once-duration-override-operator
    ```

 1. open the console Operators -> OperatorHub, search for  `Run Once Duration Override Operator` and install the operator


 1. create CR for the Run Once Duration Override Operator in the console:
    ```
    apiVersion: operator.openshift.io/v1
    kind: RunOnceDurationOverride
    metadata:
      name: cluster
    spec:
      runOnceDurationOverride:
        spec:
          activeDeadlineSeconds: 3600
    ```
