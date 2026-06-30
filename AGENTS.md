# AGENTS.md

OpenShift operator that manages an admission webhook server to automatically override activeDeadlineSeconds for run-once pods.

## What This Operator Does

This operator manages the RunOnceDurationOverride admission webhook server in OpenShift clusters:

- **Admission Webhook Server** (`openshift-run-once-duration-override-operator` namespace) - DaemonSet running on master nodes that intercepts pod creation
- **Active Deadline Override** - Automatically sets `activeDeadlineSeconds` on pods with `restartPolicy: Never` or `restartPolicy: OnFailure`
- **Certificate Management** - Generates and rotates self-signed serving certificates for the webhook server
- **CRD Management** - Watches and reconciles `RunOnceDurationOverride` custom resources (singleton named "cluster")
- **Namespace Filtering** - Only applies to namespaces with label `runoncedurationoverrides.admission.runoncedurationoverride.openshift.io/enabled: "true"`

The operator prevents run-once pods (Jobs, CronJobs, init containers) from running indefinitely by enforcing a maximum execution time.

## Commands

```bash
# Build and verify
make build
make verify

# Testing
make test-unit                # Fast unit tests  
make test-e2e                 # E2E tests (requires cluster)

# Update CRDs after modifying API types
make regen-crd                # Regenerate CRD from Go types

# Code generation
make generate                 # Generate clients and update CRDs
make generate-clients         # Only generate clientset/informers/listers

# Dependencies
go get <module>@<version>
go mod tidy
make verify
```

## Tech Stack

- **Go** - Version 1.25+ (check `go.mod`)
- **Kubernetes client-go** - v0.35.2
- **OpenShift library-go** - Controller factory, resourceapply, v1helpers
- **OpenShift api** - operator/v1 for CRD types
- **klog/v2** - Structured logging
- **Cobra** - CLI framework
- **controller-gen** - CRD generation from Go types with kubebuilder markers
- **OpenShift Tests Extension (OTE)** - E2E test framework

## Always Do

- **Use informers and listers** - Never make direct API calls in controller sync loops
- **Run `make regen-crd`** after modifying API types in `pkg/apis/runoncedurationoverride/v1/`
- **Use handler chain pattern** - Controllers use sequential handlers for validation, cert generation, deployment, etc.
- **Return typed errors** - Use `NewInstallReadinessError()`, `NewConfigurationError()`, etc. for proper condition reporting
- **Use resourceapply helpers** - `ApplyConfigMap()`, `ApplySecret()`, etc. from library-go
- **Hash configuration** - Update hash annotations when ConfigMap or Secret content changes
- **Validate activeDeadlineSeconds** - Must be >= 0 (validated in `RunOnceDurationOverrideConfigSpec.Validate()` in `pkg/apis/runoncedurationoverride/v1/override.go:29`)
- **Log with klog** - Use structured logging with klog.V() levels (V(4) for debug, V(1) for info)
- **Reference files with line numbers** - Format: `pkg/operator/targetconfigcontroller/handler_deploy.go:45`

## Ask First

- **Changing webhook admission behavior** - Impacts all run-once pods cluster-wide in labeled namespaces
- **Modifying CRD schema** - Breaking changes require upgrade path and deprecation
- **Changing operator namespace** - Fixed to `openshift-run-once-duration-override-operator`
- **Certificate rotation timing** - Currently uses self-signed certs with specific expiration logic
- **DaemonSet scheduling** - Runs only on master nodes with hostNetwork for API server access
- **Default activeDeadlineSeconds value** - Currently 800 seconds (13.3 minutes) in example CR

## Never Do

- **Never commit secrets** or credentials to the repo
- **Never modify `vendor/`** directly - Use `go get` and `go mod tidy`
- **Never make direct API calls in controllers** - Always use informers/listers for performance
- **Never modify API types without running `make regen-crd`**
- **Never skip certificate validation** - Webhook server requires valid TLS certificates
- **Never apply webhook to all namespaces** - Must be opt-in via namespace label
- **Never set activeDeadlineSeconds < 0** - Invalid configuration will be rejected
- **Never skip hash annotations** - Required for tracking ConfigMap/Secret changes

## Controller Pattern

The operator uses a handler chain pattern with library-go factory:

```go
import (
    "github.com/openshift/library-go/pkg/controller/factory"
    "github.com/openshift/library-go/pkg/operator/events"
    runoncedurationoverridev1 "github.com/openshift/run-once-duration-override-operator/pkg/apis/runoncedurationoverride/v1"
)

// Handler chain in targetconfigcontroller.NewTargetConfigController()
handlers := []Handler{
    NewAvailabilityHandler(operandAsset, deployInterface),
    NewValidationHandler(),
    NewConfigurationHandler(kubeClient, recorder, ...),
    NewCertGenerationHandler(kubeClient, recorder, ...),
    NewCertReadyHandler(kubeClient, ...),
    NewDaemonSetHandler(kubeClient, recorder, ...),
    NewDeploymentReadyHandler(deployInterface),
    NewWebhookConfigurationHandlerHandler(kubeClient, recorder, ...),
    NewAvailabilityHandler(operandAsset, deployInterface),
}

// Each handler implements (defined in handler_context.go:23):
type Handler interface {
    Handle(context *ReconcileRequestContext, original *RunOnceDurationOverride) (
        current *RunOnceDurationOverride, 
        result controllerreconciler.Result, 
        handleErr error,
    )
}
```

## Handler Chain Execution Order

Located in `pkg/operator/targetconfigcontroller/controller.go:62-72`:

1. **AvailabilityHandler** - Initial check for deployment availability
2. **ValidationHandler** - Validates `activeDeadlineSeconds > 0`
3. **ConfigurationHandler** - Creates/updates ConfigMap with webhook config
4. **CertGenerationHandler** - Generates serving certificates and CA bundle
5. **CertReadyHandler** - Verifies certificates are populated and valid
6. **DaemonSetHandler** - Creates/updates webhook server DaemonSet
7. **DeploymentReadyHandler** - Waits for DaemonSet pods to be ready
8. **WebhookConfigurationHandler** - Creates MutatingWebhookConfiguration
9. **AvailabilityHandler** - Final availability check and status update

## Configuration Example

```yaml
apiVersion: operator.openshift.io/v1
kind: RunOnceDurationOverride
metadata:
  name: cluster  # Must be named "cluster" (singleton)
spec:
  managementState: Managed  # or Removed
  runOnceDurationOverride:
    spec:
      activeDeadlineSeconds: 3600  # Override value in seconds (must be > 0)
```

## Webhook Behavior

The webhook server (separate repository: [run-once-duration-override](https://github.com/openshift/run-once-duration-override)) intercepts pod creation and:

1. Checks if pod's namespace has label `runoncedurationoverrides.admission.runoncedurationoverride.openshift.io/enabled: "true"`
2. Checks if pod has `restartPolicy: Never` or `restartPolicy: OnFailure`
3. If both conditions met, sets `pod.spec.activeDeadlineSeconds` to configured value
4. Only applies if `activeDeadlineSeconds` is not already set by the user

## Error Types and Conditions

Defined in `pkg/apis/runoncedurationoverride/v1/override_types.go:15-24`:

- **InstallReadinessFailure** - General installation/readiness issues
- **InvalidParameters** - activeDeadlineSeconds validation failed
- **ConfigurationCheckFailed** - ConfigMap generation/application failed
- **CertNotAvailable** - Serving certificates missing or invalid
- **CannotSetReference** - Failed to set object reference in status
- **CannotGenerateCert** - Certificate generation failed
- **InternalError** - Unexpected controller errors
- **AdmissionWebhookNotAvailable** - MutatingWebhookConfiguration not ready
- **DeploymentNotReady** - DaemonSet pods not running

## Common Workflows

### Adding a New Handler

1. Create handler file: `pkg/operator/targetconfigcontroller/handler_myfeature.go`
2. Implement `Handler` interface with `Handle()` method
3. Add handler to chain in `controller.go:62-72`
4. Return typed errors using `NewInstallReadinessError()` or similar
5. Update tests in `handler_conditions_test.go`

### Modifying the CRD

1. Edit `pkg/apis/runoncedurationoverride/v1/override_types.go`
2. Add kubebuilder markers if needed (e.g., `+kubebuilder:validation:Minimum=1`)
3. Run `make regen-crd` to regenerate CRD YAML
4. CRD is copied to `manifests/runoncedurationoverride.crd.yaml`, `test/e2e/bindata/assets/08_crd.yaml`, and `deploy/02_runoncedurationoverride.crd.yaml`
5. Run `make verify` to check codegen is up to date

### Updating Webhook Server Image

The operand (webhook server) image is specified via environment variable:

```bash
# In deployment manifest (deploy/07_deployment.yaml)
env:
  - name: RELATED_IMAGE_OPERAND_IMAGE
    value: "quay.io/openshift/run-once-duration-override:latest"
```

Operator reads this environment variable in `pkg/operator/start.go:46-58` and passes it to the webhook DaemonSet.

## Key Files and Locations

### API Types
- `pkg/apis/runoncedurationoverride/v1/override_types.go` - CRD definition and spec/status types

### Controller
- `pkg/operator/start.go` - Main operator startup and controller initialization
- `pkg/operator/targetconfigcontroller/controller.go` - Reconciliation controller with handler chain
- `pkg/operator/targetconfigcontroller/handler_*.go` - Individual handler implementations

### Asset Generation
- `pkg/asset/asset.go` - Asset values (names, labels, annotations)
- `pkg/asset/daemonset.go` - Webhook server DaemonSet template
- `pkg/asset/webhookconfiguration.go` - MutatingWebhookConfiguration template
- `pkg/asset/configuration.go` - ConfigMap template for webhook config
- `pkg/asset/rbac.go` - ServiceAccount, Role, ClusterRole templates

### Certificate Management
- `pkg/cert/cert.go` - Certificate validation helpers
- `pkg/cert/x509.go` - X.509 certificate generation
- `pkg/operator/targetconfigcontroller/handler_cert_generation.go` - Cert generation handler

### Deployment
- `pkg/deploy/deploy.go` - Deploy interface for DaemonSet management
- `pkg/deploy/daemonset.go` - DaemonSet install implementation

### Tests
- `test/e2e/operator.go` - E2E tests using OpenShift Tests Extension (OTE)
- `test/e2e/operator_test.go` - Standard Go test wrapper for E2E tests
- `pkg/operator/targetconfigcontroller/*_test.go` - Unit tests

## Debugging Tips

1. **Check operator logs**:
   ```bash
   oc logs -n openshift-run-once-duration-override-operator deployment/run-once-duration-override-operator
   ```

2. **Check webhook server logs**:
   ```bash
   oc logs -n openshift-run-once-duration-override-operator -l runoncedurationoverride=true
   ```

3. **Verify MutatingWebhookConfiguration**:
   ```bash
   oc get mutatingwebhookconfiguration runoncedurationoverrides.admission.runoncedurationoverride.openshift.io -o yaml
   ```

4. **Check CR status**:
   ```bash
   oc get runoncedurationoverride cluster -o yaml
   ```

5. **Enable debug logging**: Set `--v=4` in DaemonSet args for verbose logging

## OpenShift Tests Extension (OTE) Framework

This operator uses OTE for E2E testing. Key points:

- Tests are in `test/e2e/operator.go` with Ginkgo specs
- Also provides standard Go test wrapper in `test/e2e/operator_test.go`
- Build test binary: `make build` (creates `run-once-duration-override-operator-tests-ext`)
- List suites: `./run-once-duration-override-operator-tests-ext list suites`
- Run tests: `./run-once-duration-override-operator-tests-ext run-suite openshift/run-once-duration-override-operator/operator/serial`

Test suite path: `openshift/run-once-duration-override-operator/operator/serial`
