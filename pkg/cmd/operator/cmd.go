package operator

import (
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

	return cmd
}
