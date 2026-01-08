package e2e

import (
	"testing"

	o "github.com/onsi/gomega"
)

// NOTE: This test is also available in the OTE framework (test/e2e/operator.go).
// This dual implementation allows tests to run both as standard Go tests (via go test)
// and through the Ginkgo/OTE framework (for OpenShift CI integration).
//
// The actual test logic is in operator.go's standalone functions, which are called
// by both this standard Go test and the Ginkgo specs.

// TestExtended runs the operator tests using standard Go testing.
func TestExtended(t *testing.T) {
	// Register Gomega with the testing framework for standard Go test mode
	o.RegisterTestingT(t)

	t.Run("RunOnceDurationOverride Operator", func(t *testing.T) {
		// Setup operator and wait for it to be ready
		ctx, cancelFnc, kubeClient, err := setupOperator(t)
		if err != nil {
			t.Fatalf("Failed to setup operator: %v", err)
		}
		defer cancelFnc()

		t.Run("ActiveDeadlineSeconds webhook", func(t *testing.T) {
			testNamespace := testActiveDeadlineSecondsWebhook(t, ctx, kubeClient)
			defer cleanupTestNamespace(t, ctx, kubeClient, testNamespace)
		})
	})
}
