# Architecture - Run Once Duration Override Operator

## Overview

The Run Once Duration Override Operator manages a mutating admission webhook server that automatically sets `activeDeadlineSeconds` on run-once pods in OpenShift clusters. It reconciles a singleton `RunOnceDurationOverride` custom resource and deploys/configures the webhook server to prevent run-once pods (Jobs, CronJobs) from running indefinitely.

**Key Responsibilities:**
- Deploy and manage the webhook server DaemonSet on master nodes
- Generate and rotate TLS serving certificates for webhook server
- Create and configure MutatingWebhookConfiguration for pod admission
- Manage webhook configuration via ConfigMap
- Report operator status via CR status conditions
- Support namespace-based opt-in for webhook application

## High-Level Architecture

```
┌─────────────────────────────────────────────────────────────────────┐
│                    OpenShift Cluster                                │
│                                                                     │
│  ┌──────────────────────────────────────────────────────────────┐  │
│  │  openshift-run-once-duration-override-operator namespace     │  │
│  │                                                               │  │
│  │  ┌────────────────────────────────────┐                      │  │
│  │  │  Operator Deployment (1 replica)   │                      │  │
│  │  │  ┌──────────────────────────────┐  │                      │  │
│  │  │  │  targetconfigcontroller      │  │                      │  │
│  │  │  │  - Watches RunOnceDuration   │  │                      │  │
│  │  │  │    Override CR               │  │                      │  │
│  │  │  │  - Handler chain execution:  │  │                      │  │
│  │  │  │    1. Validation             │  │                      │  │
│  │  │  │    2. Configuration          │  │                      │  │
│  │  │  │    3. Cert Generation        │  │                      │  │
│  │  │  │    4. DaemonSet Deploy       │  │                      │  │
│  │  │  │    5. Webhook Registration   │  │                      │  │
│  │  │  │    6. Availability Check     │  │                      │  │
│  │  │  └──────────────────────────────┘  │                      │  │
│  │  └────────────────────────────────────┘                      │  │
│  │           │                                                   │  │
│  │           │ creates/manages                                   │  │
│  │           ▼                                                   │  │
│  │  ┌────────────────────────────────────┐                      │  │
│  │  │  Webhook Server DaemonSet          │                      │  │
│  │  │  - Runs on all master nodes        │                      │  │
│  │  │  - HostNetwork: true               │                      │  │
│  │  │  - Listens on 127.0.0.1:9448       │                      │  │
│  │  │  - Mutates pod specs               │                      │  │
│  │  │  - Sets activeDeadlineSeconds      │                      │  │
│  │  └────────────────────────────────────┘                      │  │
│  │                                                               │  │
│  │  Resources:                                                   │  │
│  │  - ConfigMap: runoncedurationoverride-configuration          │  │
│  │  - Secret: server-serving-cert-runoncedurationoverride       │  │
│  │  - ConfigMap: runoncedurationoverride-service-serving (CA)   │  │
│  │  - Service: runoncedurationoverride (webhook endpoint)       │  │
│  └──────────────────────────────────────────────────────────────┘  │
│                                                                     │
│  ┌──────────────────────────────────────────────────────────────┐  │
│  │  Cluster-wide Resources                                       │  │
│  │                                                                │  │
│  │  - RunOnceDurationOverride CR (singleton: "cluster")         │  │
│  │  - MutatingWebhookConfiguration:                             │  │
│  │    runoncedurationoverrides.admission...openshift.io         │  │
│  └──────────────────────────────────────────────────────────────┘  │
│                                                                     │
│  ┌──────────────────────────────────────────────────────────────┐  │
│  │  API Server                                                   │  │
│  │  - Routes pod CREATE to webhook                              │  │
│  │  - Only for namespaces with label:                           │  │
│  │    runoncedurationoverrides.admission...enabled=true         │  │
│  └──────────────────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────────────────┘
```

## Components

### 1. Operator Controller (`targetconfigcontroller`)

**Purpose**: Main reconciliation controller that watches the `RunOnceDurationOverride` CR and ensures all webhook components are deployed and configured correctly.

**Location**: `pkg/operator/targetconfigcontroller/controller.go`

**Key Functions:**
- Watches singleton `RunOnceDurationOverride` CR named "cluster"
- Executes handler chain for validation, configuration, and deployment
- Updates CR status with conditions and resource references
- Manages informers for all secondary resources (ConfigMaps, Secrets, DaemonSets, etc.)

**Reconciliation Flow:**
1. Read `RunOnceDurationOverride/cluster` CR from lister
2. Deep copy CR for modification
3. Execute handler chain sequentially:
   - Each handler receives modified CR from previous handler
   - Handlers can set status fields and return errors
   - If handler returns error, chain stops
   - If handler requests requeue, chain stops
4. Collect all status updates from handlers
5. Update CR status via status subresource
6. Return error for automatic retry or nil for success

**Controller Factory Pattern** (`controller.go:75-86`):
```go
factory.New().WithFilteredEventsInformers(
    isOwnedByOperator,  // Filter: only resources owned by "cluster" CR
    // Watch these resources for changes:
    operatorInformerFactory.RunOnceDurationOverride().V1().RunOnceDurationOverrides().Informer(),
    informerFactory.Apps().V1().DaemonSets().Informer(),
    informerFactory.Core().V1().ConfigMaps().Informer(),
    informerFactory.Core().V1().Secrets().Informer(),
    informerFactory.Admissionregistration().V1().MutatingWebhookConfigurations().Informer(),
    // ... etc
).WithSync(c.sync).ToController(ControllerName, recorder)
```

### 2. Handler Chain Pattern

**Purpose**: Modular reconciliation logic split into discrete handlers for validation, resource creation, and status updates.

**Location**: `pkg/operator/targetconfigcontroller/handler*.go`

**Handler Interface** (`handler.go`):
```go
type Handler interface {
    Handle(context *ReconcileRequestContext, original *RunOnceDurationOverride) (
        current *RunOnceDurationOverride,
        result controllerreconciler.Result,
        handleErr error,
    )
}
```

**Handler Execution Order** (`controller.go:62-72`):

1. **AvailabilityHandler** (`handler_availability.go`)
   - Initial check: is webhook DaemonSet available?
   - Sets preliminary availability status
   - Returns early if deployment not ready (allows other handlers to run)

2. **ValidationHandler** (`handler_validation.go`)
   - Validates `spec.runOnceDurationOverride.spec.activeDeadlineSeconds > 0`
   - Returns `InvalidParameters` error if validation fails
   - Prevents deployment of invalid configuration

3. **ConfigurationHandler** (`handler_configuration.go`)
   - Creates/updates ConfigMap `runoncedurationoverride`
   - Contains webhook configuration (activeDeadlineSeconds value)
   - Calculates hash of configuration content
   - Updates `status.hash.configuration` and `status.resources.configurationRef`
   - Format: YAML with key `configuration.yaml`

4. **CertGenerationHandler** (`handler_cert_generation.go`)
   - Generates self-signed TLS serving certificate for webhook server
   - Creates Secret `runoncedurationoverride-serving-cert` with `tls.crt` and `tls.key`
   - Creates ConfigMap `runoncedurationoverride-cabundle` with CA certificate
   - Calculates cert rotation time (certificates have limited lifetime)
   - Updates `status.certsRotateAt`, `status.hash.servingCert`
   - Returns `CannotGenerateCert` error on failure

5. **CertReadyHandler** (`handler_cert_ready.go`)
   - Verifies serving cert Secret is populated with valid `tls.crt` and `tls.key`
   - Returns `CertNotAvailable` error if cert missing or invalid
   - Does not generate certs (that's CertGenerationHandler's job)

6. **DaemonSetHandler** (`handler_deploy.go`)
   - Creates/updates webhook server DaemonSet
   - Uses `pkg/asset/daemonset.go` template
   - Applies configuration hash and serving cert hash as pod annotations
   - Triggers pod restart when config or certs change (via annotation change)
   - Updates `status.resources.deploymentRef`, `status.image`
   - Uses `deploy.Interface` for DaemonSet management

7. **DeploymentReadyHandler** (`handler_deployment_ready.go`)
   - Checks if DaemonSet is available (ready replicas > 0)
   - Returns `DeploymentNotReady` error if not ready
   - Queries DaemonSet lister for current state

8. **WebhookConfigurationHandler** (`handler_webhook.go`)
   - Creates/updates MutatingWebhookConfiguration
   - Injects CA bundle from `runoncedurationoverride-cabundle` ConfigMap
   - Configures webhook rules: only pods in labeled namespaces
   - Sets failurePolicy, matchPolicy, sideEffects, etc.
   - Updates `status.resources.mutatingWebhookConfigurationRef`
   - Returns `AdmissionWebhookNotAvailable` error on failure

9. **AvailabilityHandler** (again)
   - Final availability check after all resources deployed
   - Sets `Available` condition to `True` if everything ready
   - Updates operator status for ClusterOperator reporting

**Handler Context** (`handler_context.go`):
```go
type ReconcileRequestContext struct {
    OperandContext operatorruntime.OperandContext  // Names, namespace, images
}
```

### 3. Webhook Server DaemonSet

**Purpose**: Runs the admission webhook server on all master nodes to intercept pod creation requests.

**Location**: Template in `pkg/asset/daemonset.go`

**Key Characteristics:**
- **Deployment Type**: DaemonSet (not Deployment) - runs on every master node for HA
- **Node Selector**: `node-role.kubernetes.io/master: ""`
- **Host Network**: `hostNetwork: true` - allows listening on 127.0.0.1 (localhost of node)
- **Container Port**: 9448 on localhost (not exposed cluster-wide)
- **Image**: Specified via `RELATED_IMAGE_OPERAND_IMAGE` env var in operator
- **Security**:
  - `runAsNonRoot: true`
  - `allowPrivilegeEscalation: false`
  - `readOnlyRootFilesystem: true`
  - Capabilities dropped: ALL
  - Seccomp: RuntimeDefault
- **Tolerations**: Tolerates master node taints and temporary node unreachability

**Container Args** (`daemonset.go:69-78`):
```bash
/usr/bin/run-once-duration-override \
  --secure-port=9448 \
  --bind-address=127.0.0.1 \
  --tls-cert-file=/var/serving-cert/tls.crt \
  --tls-private-key-file=/var/serving-cert/tls.key \
  --v=3
```

**Environment Variables**:
- `CONFIGURATION_PATH=/etc/runoncedurationoverride/config/override.yaml` - path to config file

**Volume Mounts**:
- `/var/serving-cert` - Secret with TLS cert/key
- `/etc/runoncedurationoverride/config/override.yaml` - ConfigMap with activeDeadlineSeconds value

**Upstream Webhook Server**:
The webhook server itself is in a separate repository: [github.com/openshift/run-once-duration-override](https://github.com/openshift/run-once-duration-override)

### 4. Asset System

**Purpose**: Centralized template system for generating Kubernetes resources with consistent naming, labels, and configuration.

**Location**: `pkg/asset/`

**Asset Values** (`asset.go:10-34`):
```go
type Values struct {
    Name                 string  // "runoncedurationoverride"
    Namespace            string  // "openshift-run-once-duration-override-operator"
    ServiceAccountName   string
    OperandImage         string  // Webhook server image
    OperandVersion       string
    AdmissionAPIGroup    string  // "admission.runoncedurationoverride.openshift.io"
    OwnerLabelKey        string  // For filtering owned resources
    SelectorLabelKey     string  // For pod selection in DaemonSet
    ConfigurationKey     string  // "configuration.yaml"
    // Hash annotation keys for triggering pod restarts
    ConfigurationHashAnnotationKey  string
    ServingCertHashAnnotationKey    string
    ObservedConfigHashAnnotationKey string
}
```

**Asset Templates**:
- `daemonset.go` - Webhook server DaemonSet
- `configuration.go` - ConfigMap with webhook config
- `secret.go` - Secret for serving certificates
- `ca_bundle_configmap.go` - ConfigMap for CA bundle
- `service.go` - Service for webhook endpoint
- `webhookconfiguration.go` - MutatingWebhookConfiguration
- `rbac.go` - ClusterRole, Role, ClusterRoleBinding, RoleBinding
- `sa.go` - ServiceAccount

### 5. Certificate Management

**Purpose**: Generate and manage TLS certificates for secure webhook server communication.

**Location**: `pkg/cert/`

**Certificate Flow**:
1. Operator generates self-signed CA certificate (`cert/x509.go:GenerateCA()`)
2. Operator generates serving certificate signed by CA (`cert/x509.go:CreateSignedServingPair()`)
3. Serving cert/key stored in Secret `runoncedurationoverride-serving-cert`
4. CA cert stored in ConfigMap `runoncedurationoverride-cabundle`
5. MutatingWebhookConfiguration references CA bundle for validation
6. Webhook server loads serving cert/key from mounted Secret

**Certificate Validation** (`cert/cert.go:48-63`):
```go
func IsPopulated(secret *corev1.Secret) bool {
    if secret == nil || len(secret.Data) == 0 {
        return false
    }
    if len(secret.Data["tls.key"]) == 0 || len(secret.Data["tls.crt"]) == 0 {
        return false
    }
    return true
}
```

**Certificate Rotation**:
- Certificates have expiration time
- `status.certsRotateAt` tracks rotation time
- Controller regenerates certs when rotation time approaches

### 6. Deployment Interface

**Purpose**: Abstract interface for DaemonSet operations (create, update, check availability).

**Location**: `pkg/deploy/`

**Interface** (`deploy.go:18-23`):
```go
type Interface interface {
    Name() string
    IsAvailable() (available bool, err error)
    Get() (object runtime.Object, accessor metav1.Object, err error)
    Ensure(parent, child Applier, generations []operatorsv1.GenerationStatus) (
        object runtime.Object, accessor metav1.Object, err error)
}
```

**Implementation** (`deploy/daemonset.go`):
- `DaemonSetInstall` - Manages DaemonSet lifecycle
- Uses library-go `resourceapply` helpers
- Compares desired vs actual DaemonSet state
- Applies updates when configuration changes

## Data Flow

### Webhook Request Flow

```
1. User creates Pod with restartPolicy: OnFailure
   ↓
2. Kube API Server receives CREATE request
   ↓
3. API Server checks MutatingWebhookConfiguration
   - Matches namespace label?
   - Matches pod CREATE operation?
   ↓
4. API Server calls webhook: POST https://127.0.0.1:9448/apis/admission.../runoncedurationoverrides
   ↓
5. Webhook server (running on master node via DaemonSet):
   - Reads configuration from /etc/runoncedurationoverride/config/override.yaml
   - Checks pod.spec.restartPolicy (Never or OnFailure?)
   - Checks if activeDeadlineSeconds already set
   - If not set, patches pod.spec.activeDeadlineSeconds = <configured value>
   ↓
6. Webhook returns admission response with patch
   ↓
7. API Server applies patch to pod
   ↓
8. Pod is created with activeDeadlineSeconds set
```

### Operator Reconciliation Flow

```
1. User creates/updates RunOnceDurationOverride CR
   ↓
2. Informer detects change, enqueues "cluster" key
   ↓
3. Controller sync() function called
   ↓
4. Lister retrieves RunOnceDurationOverride/cluster
   ↓
5. Handler chain executes:
   - ValidationHandler: check activeDeadlineSeconds > 0
   - ConfigurationHandler: create/update ConfigMap
   - CertGenerationHandler: generate/update certificates
   - CertReadyHandler: verify certificates valid
   - DaemonSetHandler: create/update webhook DaemonSet
   - DeploymentReadyHandler: check DaemonSet ready
   - WebhookConfigurationHandler: create/update MutatingWebhookConfiguration
   - AvailabilityHandler: set Available condition
   ↓
6. Update CR status with conditions and resource references
   ↓
7. If error: set Degraded condition, return error (requeue)
8. If success: set Available=True, return nil
```

### Configuration Update Flow

```
1. User updates RunOnceDurationOverride CR:
   spec.runOnceDurationOverride.spec.activeDeadlineSeconds: 3600 → 7200
   ↓
2. Controller reconciles
   ↓
3. ConfigurationHandler:
   - Generates new ConfigMap content with activeDeadlineSeconds: 7200
   - Calculates new configuration hash
   - Updates ConfigMap
   - Updates status.hash.configuration
   ↓
4. DaemonSetHandler:
   - Sees configuration hash changed
   - Updates DaemonSet pod template annotation with new hash
   - Kubernetes detects DaemonSet change
   - Rolls out new DaemonSet pods (one at a time, master by master)
   ↓
5. New webhook pods start:
   - Mount updated ConfigMap
   - Read new activeDeadlineSeconds value
   - Apply new value to subsequent pod creation requests
```

## Status Reporting

### CR Status Fields

**Location**: `pkg/apis/runoncedurationoverride/v1/override_types.go:65-76`

```go
type RunOnceDurationOverrideStatus struct {
    operatorsv1.OperatorStatus  // Standard operator conditions, generations, etc.

    Resources RunOnceDurationOverrideResources  // Object references
    Hash      RunOnceDurationOverrideResourceHash  // Configuration/cert hashes
    Image     string  // Operand image being used
    CertsRotateAt metav1.Time  // When certificates will be rotated
}
```

**Resource References** (`override_types.go:84-114`):
- `ConfigurationRef` - ConfigMap with webhook config
- `ServiceCAConfigMapRef` - ConfigMap with CA bundle
- `ServiceRef` - Service for webhook endpoint
- `ServiceCertSecretRef` - Secret with serving cert
- `DeploymentRef` - DaemonSet reference
- `MutatingWebhookConfigurationRef` - MutatingWebhookConfiguration reference

**Hash Fields** (`override_types.go:78-82`):
- `Configuration` - Hash of ConfigMap content (triggers pod restart on change)
- `ServingCert` - Hash of serving cert (triggers pod restart on rotation)
- `ObservedConfig` - Hash of observed configuration (for config observation pattern)

### Condition Types

**Location**: `pkg/apis/runoncedurationoverride/v1/override_types.go:15-24`

- **InstallReadinessFailure** - General installation/readiness issues
- **InvalidParameters** - `activeDeadlineSeconds <= 0` or other validation failures
- **ConfigurationCheckFailed** - ConfigMap creation/update failed
- **CertNotAvailable** - Serving certificates missing or invalid
- **CannotSetReference** - Failed to set object reference in status
- **CannotGenerateCert** - Certificate generation failed
- **InternalError** - Unexpected errors in controller logic
- **AdmissionWebhookNotAvailable** - MutatingWebhookConfiguration creation failed
- **DeploymentNotReady** - DaemonSet pods not running/ready
- **Available** - All components deployed and ready (standard operator condition)
- **Progressing** - Deployment in progress (standard operator condition)
- **Degraded** - Operator in degraded state (standard operator condition)

### Error Handling

**Typed Errors** (`targetconfigcontroller/error.go`):

Handlers return typed errors that map to specific condition types:

```go
func NewInstallReadinessError(reason string, err error) error
func NewConfigurationError(reason string, err error) error
// ... etc
```

Controller converts these errors to conditions in `sync()` function (`controller.go:140-161`).

## Design Decisions

### Why DaemonSet Instead of Deployment?

**Decision**: Use DaemonSet for webhook server instead of Deployment with multiple replicas.

**Rationale**:
- Webhook must be available on every master node for HA
- API server accesses webhook via localhost (127.0.0.1:9448) on the same node
- HostNetwork required to bind to localhost
- DaemonSet ensures one pod per master node automatically
- Scales automatically as masters are added/removed

**Trade-offs**:
- More resource usage (one pod per master vs. 3 replicas)
- Rolling updates slower (one node at a time)
- But: better availability and simpler networking

### Why Handler Chain Pattern?

**Decision**: Split reconciliation logic into sequential handlers instead of monolithic sync function.

**Rationale**:
- Separation of concerns (validation, certs, deployment are independent)
- Easier testing (test each handler independently)
- Clear error attribution (each handler returns specific error type)
- Reusable handlers (AvailabilityHandler used twice in chain)
- Easier to add new steps (just add handler to chain)

**Trade-offs**:
- More boilerplate code
- Handlers must be carefully ordered (e.g., cert generation before deployment)
- Harder to understand overall flow (must read all handlers)

### Why Self-Signed Certificates?

**Decision**: Generate self-signed certificates instead of using service-ca operator.

**Rationale**:
- Full control over cert lifecycle and rotation
- No dependency on service-ca operator being installed
- Can customize cert validity period
- Simpler deployment (no waiting for service-ca to inject certs)

**Trade-offs**:
- Operator responsible for cert rotation
- More complex cert generation code
- Manual CA trust configuration in MutatingWebhookConfiguration

### Why Namespace Label for Opt-In?

**Decision**: Require namespace label `runoncedurationoverrides.admission.runoncedurationoverride.openshift.io/enabled: "true"` instead of applying to all namespaces.

**Rationale**:
- Prevents unintended impact on critical workloads
- Users explicitly opt-in per namespace
- Easier debugging (smaller blast radius)
- Follows OpenShift pattern for admission webhooks

**Trade-offs**:
- Extra step for users (must label namespaces)
- Not automatic for new namespaces
- But: safer and more predictable

### Why Singleton CR?

**Decision**: Only reconcile `RunOnceDurationOverride` CR named "cluster", ignore all others.

**Rationale**:
- Webhook configuration is cluster-wide (one MutatingWebhookConfiguration)
- Only one DaemonSet needed for entire cluster
- Simpler operator logic (no multi-tenancy concerns)
- Matches other OpenShift operators (KubeDescheduler, etc.)

**Implementation** (`controller.go:99-108`):
```go
original, getErr := c.lister.Get(operatorclient.OperatorConfigName)  // "cluster"
if getErr != nil {
    if k8serrors.IsNotFound(getErr) {
        klog.Errorf("object has been deleted")
        return nil  // Don't requeue
    }
    return getErr  // Requeue on other errors
}
```

## Scalability Considerations

### Webhook Server Scaling

- **Horizontal**: DaemonSet automatically scales with master node count
- **Vertical**: Adjust resource requests/limits in DaemonSet template
- **HA**: One webhook pod per master provides redundancy
- **Locality**: API server on each master calls webhook on same node (no network hop)

### Operator Scaling

- **Single replica**: Operator itself runs as single-replica Deployment
- **Leader election**: Not needed (single replica)
- **Informer caching**: Reduces API server load (uses listers for reads)
- **Event filtering**: `isOwnedByOperator()` filter reduces reconciliation triggers

### Performance

- **Webhook latency**: Localhost communication (127.0.0.1) is fast (<1ms typically)
- **Admission overhead**: Minimal (simple patch operation)
- **Reconciliation frequency**: Triggered by changes, not polling
- **Resource usage**: Low (operator ~50MB RAM, webhook ~20MB per pod)

## Security

### Webhook Server Security

- **TLS required**: All communication over HTTPS with valid certificates
- **Non-root**: Container runs as non-root user
- **Read-only filesystem**: Root filesystem is read-only
- **No privilege escalation**: Explicitly disabled
- **Capabilities dropped**: All Linux capabilities dropped
- **Seccomp profile**: RuntimeDefault

### Operator Security

- **RBAC**: Minimal permissions (only resources it manages)
- **Service Account**: Dedicated SA for operator
- **No secrets exposure**: Certificates generated in-cluster, not stored externally
- **Namespace isolation**: Runs in dedicated namespace

### Certificate Security

- **Short-lived certificates**: Certificates have limited lifetime
- **Automatic rotation**: Controller rotates certs before expiration
- **CA validation**: API server validates webhook cert against CA bundle
- **TLS 1.2+**: Modern TLS versions only

## Monitoring and Observability

### Metrics

- **Operator metrics**: Exposed on `:8080/metrics` (Prometheus format)
- **Health endpoint**: `/healthz` returns 200 OK when healthy
- **Webhook metrics**: Webhook server exposes admission webhook metrics

### Logging

- **Structured logging**: klog with verbosity levels
  - `V(1)`: Info-level logs (startup, shutdown, major events)
  - `V(4)`: Debug-level logs (reconciliation details, handler execution)
- **Log format**: Standard klog format with timestamps and log levels

### Status Conditions

- **Available**: Webhook deployed and ready
- **Progressing**: Changes being rolled out
- **Degraded**: Errors preventing operation
- **Custom conditions**: Handler-specific conditions (CertNotAvailable, etc.)

## Upgrade Strategy

### Operator Upgrade

1. New operator image deployed
2. Operator pod restarts
3. Reconciliation runs with new code
4. Existing resources updated if needed
5. DaemonSet updated if template changed

### Webhook Server Upgrade

1. Operator updated with new `RELATED_IMAGE_OPERAND_IMAGE` env var
2. Reconciliation updates DaemonSet image
3. Kubernetes performs rolling update of DaemonSet
4. One master at a time gets new webhook pod
5. API servers automatically use new webhook pods

### CRD Upgrade

- CRD updates must be backward compatible
- Use kubebuilder version markers for schema evolution
- Conversion webhooks if breaking changes needed (not currently implemented)
- `storageVersion` marker indicates stored API version

## Related Components

### Upstream Webhook Server

**Repository**: [github.com/openshift/run-once-duration-override](https://github.com/openshift/run-once-duration-override)

**Purpose**: Separate binary that implements the actual admission webhook logic.

**Why separate?**:
- Clear separation: operator manages lifecycle, webhook handles admission
- Independent versioning and releases
- Webhook can be tested independently
- Different security profiles (operator needs more permissions)

### API Types

**Repository**: [github.com/openshift/api](https://github.com/openshift/api)

**Package**: `github.com/openshift/api/operator/v1`

**Types**:
- `OperatorSpec` - Embedded in RunOnceDurationOverrideSpec
- `OperatorStatus` - Embedded in RunOnceDurationOverrideStatus
- Standard operator conditions and management state

### Library-Go

**Repository**: [github.com/openshift/library-go](https://github.com/openshift/library-go)

**Components used**:
- `controller/factory` - Controller factory pattern
- `operator/v1helpers` - Operator client helpers, condition helpers
- `operator/events` - Event recording
- `operator/resourceapply` - Resource application (idempotent create/update)

## Troubleshooting Architecture

### Issue: Webhook Not Being Called

**Check**:
1. MutatingWebhookConfiguration exists and has correct rules
2. Namespace has required label
3. Pod has `restartPolicy: Never` or `OnFailure`
4. Webhook DaemonSet pods are running on all masters
5. API server can reach webhook endpoint (localhost:9448)

**Debug**:
```bash
# Check webhook configuration
oc get mutatingwebhookconfiguration runoncedurationoverrides.admission.runoncedurationoverride.openshift.io -o yaml

# Check DaemonSet
oc get daemonset -n openshift-run-once-duration-override-operator

# Check webhook logs
oc logs -n openshift-run-once-duration-override-operator -l runoncedurationoverride=true --tail=100

# Test webhook manually
oc create -f test-pod.yaml -v=8  # Verbose logging shows admission webhook calls
```

### Issue: Certificates Invalid

**Check**:
1. Secret `runoncedurationoverride-serving-cert` has `tls.crt` and `tls.key`
2. ConfigMap `runoncedurationoverride-cabundle` has CA certificate
3. `status.certsRotateAt` not in the past
4. MutatingWebhookConfiguration has correct CA bundle

**Debug**:
```bash
# Check cert Secret
oc get secret runoncedurationoverride-serving-cert -n openshift-run-once-duration-override-operator -o yaml

# Check CA ConfigMap
oc get configmap runoncedurationoverride-cabundle -n openshift-run-once-duration-override-operator -o yaml

# Check cert rotation time
oc get runoncedurationoverride cluster -o jsonpath='{.status.certsRotateAt}'

# Force cert regeneration by deleting Secret
oc delete secret runoncedurationoverride-serving-cert -n openshift-run-once-duration-override-operator
# Operator will regenerate on next reconciliation
```

### Issue: Configuration Not Applied

**Check**:
1. ConfigMap `runoncedurationoverride` has correct `configuration.yaml` content
2. `status.hash.configuration` matches current ConfigMap content
3. DaemonSet pod template has matching hash annotation
4. DaemonSet pods have been restarted after ConfigMap change

**Debug**:
```bash
# Check ConfigMap
oc get configmap runoncedurationoverride -n openshift-run-once-duration-override-operator -o yaml

# Check hash in status
oc get runoncedurationoverride cluster -o jsonpath='{.status.hash.configuration}'

# Check DaemonSet annotation
oc get daemonset runoncedurationoverride -n openshift-run-once-duration-override-operator -o jsonpath='{.spec.template.metadata.annotations}'

# Force pod restart by deleting pods
oc delete pods -n openshift-run-once-duration-override-operator -l runoncedurationoverride=true
```
