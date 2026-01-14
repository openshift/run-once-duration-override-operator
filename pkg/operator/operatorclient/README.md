# Operator Client

This package provides the `RunOnceDurationOverrideOperatorClient` interface for interacting with the RunOnceDurationOverride custom resource.

## GetOperatorState vs GetOperatorStateWithQuorum

- [ ] **GetOperatorState()** uses the **informer** to read cached data (fast, may be stale)
- [ ] **GetOperatorStateWithQuorum()** uses the **client** to read from the API server (live, guaranteed fresh)
