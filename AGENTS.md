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
go mod tidy && go mod vendor
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
- **Return typed errors** - Use `NewInstallReadinessError()`, `NewAvailableError()`, etc. for proper condition reporting
- **Use resourceapply helpers** - `ApplyConfigMap()`, `ApplySecret()`, etc. from library-go
- **Hash configuration** - Update hash annotations when ConfigMap or Secret content changes
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

// Each handler implements:
type Handler interface {
    Handle(context *ReconcileRequestContext, original *RunOnceDurationOverride) (
        current *RunOnceDurationOverride,
        result controllerreconciler.Result,
        handleErr error,
    )
}
```

## Handler Chain Execution Order

1. **AvailabilityHandler** - Initial check for deployment availability
2. **ValidationHandler** - Validates `activeDeadlineSeconds >= 0`
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
  managementState: Managed  # or Removed, Unmanaged, Force
  runOnceDurationOverride:
    spec:
      activeDeadlineSeconds: 800  # Override value in seconds (must be >= 0; 0 disables override)
```

## Key File Locations

```
pkg/operator/start.go                                    # Operator startup and controller initialization
pkg/operator/targetconfigcontroller/controller.go         # Reconciliation controller with handler chain
pkg/operator/targetconfigcontroller/handler_*.go          # Individual handler implementations
pkg/apis/runoncedurationoverride/v1/override_types.go     # CRD definition and spec/status types
pkg/asset/                                                # Asset templates (DaemonSet, webhook, RBAC, certs)
pkg/cert/                                                 # Certificate generation and validation
pkg/deploy/                                               # DaemonSet management interface
test/e2e/                                                 # E2E tests (OTE framework)
deploy/                                                   # Development deployment manifests
manifests/                                                # OLM manifests (CSV, CRD)
```

## Common Workflows

### Adding a New Handler

1. Create handler file: `pkg/operator/targetconfigcontroller/handler_myfeature.go`
2. Implement `Handler` interface with `Handle()` method
3. Add handler to chain in `controller.go`
4. Return typed errors using `NewInstallReadinessError()` or similar
5. Add unit tests

### Modifying the CRD

1. Edit `pkg/apis/runoncedurationoverride/v1/override_types.go`
2. Add kubebuilder markers if needed
3. Run `make regen-crd` to regenerate CRD YAML
4. Run `make verify` to check codegen is up to date

### Updating Webhook Server Image

Update `RELATED_IMAGE_OPERAND_IMAGE` and `OPERAND_VERSION` env vars in `deploy/07_deployment.yaml`. The operator reads both in `pkg/operator/start.go` and requires them at startup — `RELATED_IMAGE_OPERAND_IMAGE` specifies the webhook DaemonSet image, `OPERAND_VERSION` sets the reported operand version.
