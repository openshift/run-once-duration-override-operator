package main

import (
	"os"

	"k8s.io/component-base/cli"

	"github.com/openshift/run-once-duration-override-operator/pkg/cmd/operator"
)

func main() {
	code := cli.Run(operator.NewStartCommand())
	os.Exit(code)
}
