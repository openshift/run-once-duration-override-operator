package operator

import (
	"context"
	"errors"

	"k8s.io/client-go/rest"
)

type Config struct {
	// ShutdownContext is the parent context.
	ShutdownContext context.Context

	// RestConfig is the rest.Config object to be used to build clients.
	RestConfig *rest.Config
}

func (c *Config) String() string {
	return "operator config"
}

func (c *Config) Validate() error {
	if c.RestConfig == nil {
		return errors.New("no rest.Config has been specified")
	}

	return nil
}
