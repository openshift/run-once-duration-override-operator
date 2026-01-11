package main

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"k8s.io/component-base/cli"
	"k8s.io/klog/v2"

	otecmd "github.com/openshift-eng/openshift-tests-extension/pkg/cmd"
	oteextension "github.com/openshift-eng/openshift-tests-extension/pkg/extension"
	oteginkgo "github.com/openshift-eng/openshift-tests-extension/pkg/ginkgo"
	"github.com/openshift/run-once-duration-override-operator/pkg/version"
)

func main() {
	command, err := newOperatorTestCommand(context.Background())
	if err != nil {
		klog.Fatal(err)
	}
	code := cli.Run(command)
	os.Exit(code)
}

func newOperatorTestCommand(ctx context.Context) (*cobra.Command, error) {
	registry, err := prepareOperatorTestsRegistry()
	if err != nil {
		return nil, fmt.Errorf("failed to prepare operator tests registry: %w", err)
	}

	cmd := &cobra.Command{
		Use:   "run-once-duration-override-operator-tests-ext",
		Short: "A binary used to run run-once-duration-override-operator tests as part of OTE.",
		Run: func(cmd *cobra.Command, args []string) {
			// no-op, logic is provided by the OTE framework
			if err := cmd.Help(); err != nil {
				klog.Fatal(err)
			}
		},
	}

	if v := version.Get().String(); len(v) == 0 {
		cmd.Version = "<unknown>"
	} else {
		cmd.Version = v
	}

	cmd.AddCommand(otecmd.DefaultExtensionCommands(registry)...)

	return cmd, nil
}

func prepareOperatorTestsRegistry() (*oteextension.Registry, error) {
	registry := oteextension.NewRegistry()
	extension := oteextension.NewExtension("openshift", "payload", "run-once-duration-override-operator")

	// Define test suites for organized test execution
	extension.AddSuite(oteextension.Suite{
		Name:        "openshift/run-once-duration-override-operator/operator/serial",
		Parallelism: 1,
	})

	extension.AddSuite(oteextension.Suite{
		Name: "openshift/run-once-duration-override-operator/operator/parallel",
		Qualifiers: []string{
			`!(name.contains("[Serial]"))`,
		},
	})

	extension.AddSuite(oteextension.Suite{
		Name: "openshift/run-once-duration-override-operator/all",
	})

	// Build test specs from Ginkgo tests
	specs, err := oteginkgo.BuildExtensionTestSpecsFromOpenShiftGinkgoSuite()
	if err != nil {
		return nil, fmt.Errorf("failed to build extension test specs: %w", err)
	}

	if len(specs) == 0 {
		klog.Warning("No test specs found - did you import the test packages?")
	} else {
		klog.Infof("Registering %d test specs", len(specs))
	}

	extension.AddSpecs(specs)
	registry.Register(extension)
	return registry, nil
}
