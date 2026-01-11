package e2e

import (
	"os"

	o "github.com/onsi/gomega"
	apiextclientv1 "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	k8sclient "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"

	runoncedurationoverrideclient "github.com/openshift/run-once-duration-override-operator/pkg/generated/clientset/versioned"
)

// Init is a no-op function that forces the package to be loaded
// and ensures all package-level variable initializations (including ginkgo.Describe calls) are executed
func Init() {
	// This function intentionally does nothing, but its existence and invocation
	// from main.go ensures Go doesn't optimize away the package import
}

// GetKubeClient returns a Kubernetes clientset or fails the test
func GetKubeClient() *k8sclient.Clientset {
	kubeconfig := os.Getenv("KUBECONFIG")
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	o.Expect(err).NotTo(o.HaveOccurred(), "should build kubeconfig")

	client, err := k8sclient.NewForConfig(config)
	o.Expect(err).NotTo(o.HaveOccurred(), "should create kubernetes client")

	return client
}

// GetApiExtensionClient returns an API extension clientset or fails the test
func GetApiExtensionClient() *apiextclientv1.Clientset {
	kubeconfig := os.Getenv("KUBECONFIG")
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	o.Expect(err).NotTo(o.HaveOccurred(), "should build kubeconfig")

	client, err := apiextclientv1.NewForConfig(config)
	o.Expect(err).NotTo(o.HaveOccurred(), "should create API extension client")

	return client
}

// GetRunOnceDurationOverrideClient returns a RunOnceDurationOverride clientset or fails the test
func GetRunOnceDurationOverrideClient() *runoncedurationoverrideclient.Clientset {
	kubeconfig := os.Getenv("KUBECONFIG")
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	o.Expect(err).NotTo(o.HaveOccurred(), "should build kubeconfig")

	client, err := runoncedurationoverrideclient.NewForConfig(config)
	o.Expect(err).NotTo(o.HaveOccurred(), "should create RunOnceDurationOverride client")

	return client
}
