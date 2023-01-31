package main

import (
	"github.com/openshift/run-once-duration-override-operator/pkg/cmd/operator"
	"os"

	"k8s.io/component-base/cli"
)

func main() {
	code := cli.Run(operator.NewStartCommand())
	os.Exit(code)
}
