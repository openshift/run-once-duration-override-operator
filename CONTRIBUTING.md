# Contributing to the Run Once Duration Override Operator

This document serves as a guide for contributing to the Run Once Duration Override Operator, maintained by the OpenShift Control Plane group.

The Run Once Duration Override Operator manages an admission webhook server that automatically sets `activeDeadlineSeconds` on run-once pods (Jobs, CronJobs) to prevent them from running indefinitely in OpenShift clusters.

This document is explicitly for contributions to this component repository and not for high-level feature proposals within OpenShift.

Feature proposals should follow the OpenShift Enhancement Proposal process outlined in https://github.com/openshift/enhancements/blob/master/dev-guide/feature-zero-to-hero.md#openshift-feature-development-zero-to-hero-guide. If you are looking for a review on an OpenShift Enhancement Proposal that involves changes to components maintained by the control plane group, please request a review in the [`#forum-ocp-workloads`](https://redhat.enterprise.slack.com/archives/CKJR6200N) Slack channel.

This document contains the following sections:

- [Code conventions](#code-conventions) - A collection of guidelines, style suggestions, and tips for writing code.
- [Testing guidelines](#testing-guidelines) - Guidelines and expectations for testing of contributions.
- [Pull Request process/guidelines](#pull-request-process-and-guidelines) - Guidelines and expectations of pull requests containing contributions.
- [Review expectations](#review-expectations) - Guidelines and expectations for requesting reviews and interacting with reviewers.

## Code Conventions

We largely follow the [Kubernetes Code Conventions](https://github.com/kubernetes/community/blob/main/contributors/guide/coding-conventions.md#code-conventions).

Review both the Kubernetes Code Conventions and the ones specified here. There will be some overlap. If any conventions are at odds with one another, prefer the conventions explicitly documented here.

### Bash

- Follow the [shell styleguide](https://google.github.io/styleguide/shellguide.html).
- Use [`shellcheck`](https://github.com/koalaman/shellcheck) to identify common mistakes or caveats.
- Ensure that all scripts run consistently across Linux and MacOS.

### Golang (Go)

- Review [Effective Go](https://go.dev/doc/effective_go).
- Review common [Go Code Review Comments](https://go.dev/wiki/CodeReviewComments).
- Review and avoid [Go Landmines](https://gist.github.com/lavalamp/4bd23295a9f32706a48f)
- Comment your code following the [Go comment conventions](https://go.dev/doc/comment).
    - Comments should be meaningful and add context and/or explain choices that cannot be expressed through clear code.
    - All exported types, functions, and methods must have descriptive comments.
    - All unexported types, functions, and methods should have descriptive comments.
- When adding command-line flags, use dashes/hyphens (`-`) and not underscores (`_`).
- Naming
    - Please consider package name when selecting an interface name, and avoid redundancy. For example, `storage.Interface` is better than `storage.StorageInterface`.
    - Do not use uppercase characters, underscores, or dashes in package names.
    - Please consider parent directory name when choosing a package name. For example, `pkg/controllers/autoscaler/foo.go` should say `package autoscaler` not `package autoscalercontroller`.
        - Unless there's a good reason, the package foo line should match the name of the directory in which the .go file exists.
        - Importers can use a different name if they need to disambiguate.
    - Locks should be called `lock` and should never be embedded (always `lock sync.Mutex`). When multiple locks are present, give each lock a distinct name following Go conventions: `stateLock`, `mapLock` etc.
- Context propagation
    - When a function accepts or has access to a `context.Context`, pass it through to downstream calls that accept one. Never discard a context or substitute `context.Background()`/`context.TODO()` when a context is already available.
- Error handling
    - Wrap errors with meaningful context before returning or logging them.
- When logging, follow the [Kubernetes Logging Conventions](https://github.com/kubernetes/community/blob/main/contributors/devel/sig-instrumentation/logging.md).
- Dependencies must be vendored. When making changes to dependencies, ensure you've run `go mod tidy` and `go mod vendor`.
- When patching OpenShift-maintained forks of "upstream" repositories, patches should be as small as reasonably possible and should minimize touch points with code that is likely to change and impact the rebasing process.

### General

Regardless of the programming language, make sure to take the following into consideration:
- Keep readability / maintainability in mind when writing code.
    - Clever code and abstractions are often harder to reason about after the fact. Keep clever code and abstractions to the minimum necessary to accomplish the end-goal.

### Directory and File Conventions

- Avoid package sprawl. Find an appropriate subdirectory for new packages.
    - Libraries with no appropriate home belong in new package subdirectories of `pkg/util`.
- Avoid general utility packages. Packages called "util" are suspect. Instead, derive a name that describes your desired function. For example, the utility functions dealing with waiting for operations are in the `wait` package and include functionality like `Poll`. The full name is `wait.Poll`.
- All filenames should be lowercase.
- Go source files and directories use underscores, not dashes.
    - Package directories should generally avoid using separators as much as possible. When package names are multiple words, they usually should be in nested subdirectories.

### Controller Patterns

All controllers must follow the **library-go factory pattern**:

```go
import (
    "github.com/openshift/library-go/pkg/controller/factory"
    "github.com/openshift/library-go/pkg/operator/events"
    "github.com/openshift/library-go/pkg/operator/v1helpers"
    "k8s.io/client-go/informers"
)

func NewMyController(
    operatorClient v1helpers.OperatorClient,
    kubeInformers informers.SharedInformerFactory,
    recorder events.Recorder,
) factory.Controller {
    c := &myController{
        operatorClient: operatorClient,
        lister:        kubeInformers.Apps().V1().Deployments().Lister(),
    }

    return factory.New().
        WithFilteredEventsInformers(filterFunc, kubeInformers.Apps().V1().Deployments().Informer()).
        WithSync(c.sync).
        ToController("MyController", recorder)
}
```

**Key rules:**
- Use informers and listers, never direct API calls in sync loops
- Return errors from `sync()` to trigger automatic retry
- Use `resourceapply` helpers from library-go
- Register new controllers in `pkg/operator/start.go`

## Testing Guidelines

These are high-level testing guidelines. Individual component repositories may have additional testing guidelines to follow when making contributions.

- All changes must include unit test additions/changes (exceptions at reviewer/approver discretion).
- Table-driven tests are preferred for testing multiple scenarios/inputs.
- Unit tests must pass on all platforms (at the very least, Linux + MacOS).
- Co-locate tests with source code (`*_test.go`).
- Significant features should come with integration and/or end-to-end tests where appropriate.
    - End-to-end tests _may_ be scoped as a separate work item when they must be added to the openshift/origin repository (at reviewer/approver discretion).
- Do not expect asynchronous operations to happen immediately - use wait and retry patterns instead.

If necessary, manual integration testing can be done by creating a cluster using the [`Cluster Bot` Slack App](https://redhat.enterprise.slack.com/archives/D03KX7M1CRJ). Once you have a cluster created, you can follow some of the instructions in https://github.com/openshift/enhancements/blob/master/dev-guide/operators.md for guidance on how to build component images and modify cluster-operators to deploy those images.

## Pull Request Process and Guidelines

This section assumes that you have a functional understanding of `git` and how to create a pull request on GitHub.

If you do not, start with [GitHub's "Getting Started" guide](https://docs.github.com/en/get-started/start-your-journey).

### Prerequisites

Before you commit any changes or create any pull requests, you must adhere to OpenShift contribution policies. Currently, that means enabling commit signature verification.

See https://docs.google.com/document/d/1184EPSGunUkcSQYUK8T4a6iyawwi6f2zxdbB2jtG9nQ/edit?usp=sharing for more details on how to adhere to the commit signature verification policy of OpenShift.

### Creating a Pull Request

When creating a pull request, include the following:

- A brief, but descriptive, title.
    - All pull requests _should_ link to a Jira ticket associated with the work. There is automation that performs this linking when prefixing the title with the Jira ticket identifier like: `CNTRLPLANE-XXXX: my pull request title`. For pull requests that have no Jira ticket associated with it, you can prefix it with `NO-JIRA:` to signal that there is not a Jira ticket associated with it.
- A useful description of the changes being made and why they are important. Include links to supporting documents and any additional context that reviewers may need.

### CI / CD

For CI/CD, OpenShift uses Prow to run various checks. This can include unit tests, e2e tests, linters, etc.

The jobs configured for each repository are in https://github.com/openshift/release/tree/main/ci-operator/config/openshift.

There are often a mixture of required and optional checks as well as merge criteria that must be met before a pull request can merge. When any of these checks fail, the GitHub Prow bot will leave a comment on the PR with links to the run of that check that failed.

As the PR author, it is your responsibility to evaluate the failed checks and determine if there are any changes necessary to pass the checks. If you suspect that the check failure was a flake, you can trigger retests by commenting `/retest` (or `/retest-required` for retesting only the required checks) on the PR.

### Verifying Your Changes / Creating an OpenShift Cluster from a PR

As part of merging a PR, there is a requirement to verify that the changes you've made are working as expected using the `/verified` comment command.

While there are a lot of scenarios where the existing CI/CD checks may be sufficient to verify your changes are working (and can be denoted by commenting `/verified by ci`), there may be scenarios where manual verification is required.

You can use the `Cluster Bot` Slack App to create a cluster from a PR by sending it a message in the format of `launch ${OCP_VERSION},${PR_LINK} ${PLATFORM},${VARIANT}`. As an example, `launch 4.23,https://github.com/openshift/run-once-duration-override-operator/pull/123 aws,techpreview` would launch an OpenShift 4.23 cluster with the changes made in openshift/run-once-duration-override-operator#123 running on AWS with the TechPreviewNoUpgrade feature-set enabled. For more information on what `Cluster Bot` can do, you can send it a message saying `help` and it will respond with additional documentation on how it can be used.

Once you've verified your changes work as expected, you can mark the PR as verified by commenting `/verified by @{your_github_handle}` on the PR.

## Review Expectations

### Requesting a Review

If you are not a member of the OpenShift control plane team and you need a review on a PR, post it in the [#forum-ocp-workloads](https://redhat.enterprise.slack.com/archives/CKJR6200N) Slack channel or reach out to folks outlined in the OWNERS file directly.

If you are a member of the OpenShift control plane team, reviews should come from your feature team. In the event your feature team does not have someone that can approve a PR, post it in the [#control-plane](https://redhat.enterprise.slack.com/archives/CC3CZCQHM) Slack channel.

OpenShift uses AI code review tools as part of the code review process. Before requesting a review, address all feedback from the code review agent(s). It is up to your discretion as the contributor how you would like to address that feedback. Responding with an explanation as to why you are not going to take action on a comment made by the agent is an acceptable way to "address" its feedback.

### Interacting with Reviewers

When interacting with reviewers/approvers:

- Be professional.
- Be respectful of differing opinions, viewpoints, and experiences.
- Gracefully give and receive constructive feedback.
- Focus on what is best for the product/organization, not just us as individuals.

A special note on the usage of AI - to respect the time of those that are reviewing your contribution, please do not use AI to respond to review comments.

---

# Specific Guidelines for `run-once-duration-override-operator`

## Pre-Submit Checks

Before pushing your changes, run:

```sh
make build
make verify
make test-unit
```

All of these must pass for CI to succeed.

## Dependency Management

This repository vendors all Go dependencies. After adding, removing, or updating a dependency:

```sh
go mod tidy && go mod vendor
```

Always run `make verify` after vendoring to ensure consistency.

## CRD Regeneration

After modifying API types in `pkg/apis/runoncedurationoverride/v1/`:

```sh
make regen-crd
```

This updates:
- `manifests/runoncedurationoverride.crd.yaml`
- `test/e2e/bindata/assets/08_crd.yaml`
- `deploy/02_runoncedurationoverride.crd.yaml`

Always run `make verify` after regeneration to ensure consistency.

## Testing on a Cluster

For manual verification, build and push your custom image, then deploy:

```sh
export QUAY_USER=${your_quay_user_id}
export IMAGE_TAG=dev-$(git rev-parse --short HEAD)
podman build -t quay.io/${QUAY_USER}/run-once-duration-override-operator:${IMAGE_TAG} -f Dockerfile.rhel7 .
podman push quay.io/${QUAY_USER}/run-once-duration-override-operator:${IMAGE_TAG}
```

Update `RELATED_IMAGE_OPERAND_IMAGE` and `OPERAND_VERSION` in `deploy/07_deployment.yaml` with your image and version, then apply:

```sh
oc apply -f deploy/
```

Verify the webhook by creating a test namespace with label `runoncedurationoverrides.admission.runoncedurationoverride.openshift.io/enabled=true` and creating a Job. See the [README.md](README.md) for detailed deployment options.

## Debugging

```sh
# Operator logs
oc logs -n openshift-run-once-duration-override-operator deployment/run-once-duration-override-operator

# Webhook server logs
oc logs -n openshift-run-once-duration-override-operator -l runoncedurationoverride=true

# CR status
oc get runoncedurationoverride cluster -o yaml

# MutatingWebhookConfiguration
oc get mutatingwebhookconfiguration runoncedurationoverrides.admission.runoncedurationoverride.openshift.io -o yaml
```
