package operator

import (
	"context"
	"errors"
	"fmt"
	"k8s.io/client-go/rest"
)

type Config struct {
	// Name is the name of the operator. This name will be used to create kube resources.
	// More info: http://kubernetes.io/docs/user-guide/identifiers#names.
	Name string

	// ShutdownContext is the parent context.
	ShutdownContext context.Context

	// RestConfig is the rest.Config object to be used to build clients.
	RestConfig *rest.Config
}

func (c *Config) String() string {
	return fmt.Sprintf("name=%s", c.Name)
}

func (c *Config) Validate() error {
	if c.Name == "" {
		return errors.New("operator name must be specified")
	}

	if c.RestConfig == nil {
		return errors.New("no rest.Config has been specified")
	}

	return nil
}
