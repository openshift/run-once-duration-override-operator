package operator

import (
	"net/http"

	"github.com/spf13/cobra"
	"k8s.io/utils/clock"

	"github.com/openshift/library-go/pkg/controller/controllercmd"
	"github.com/openshift/run-once-duration-override-operator/pkg/operator"
	"github.com/openshift/run-once-duration-override-operator/pkg/version"
)

func NewStartCommand() *cobra.Command {
	cmd := controllercmd.
		NewControllerCommandConfig("runoncedurationoverride", version.Get(), operator.RunOperator, clock.RealClock{}).
		NewCommand()
	cmd.Use = "start"
	cmd.Short = "Start the RunOnceDurationOverride Operator"

	// Start the healthz server before the controller command runs leader
	// election so that probes pass while waiting for the lease.
	cmd.PersistentPreRun = func(cmd *cobra.Command, args []string) {
		healthMux := http.NewServeMux()
		healthMux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})
		go http.ListenAndServe(":8080", healthMux)
	}

	return cmd
}
