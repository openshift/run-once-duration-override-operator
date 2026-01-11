package runoncedurationoverride

import (
	"context"
)

// Interface defines a controller.
type Interface interface {
	// Run starts the controller and blocks until the parent context is done.
	Run(parent context.Context, errorCh chan<- error)

	// Done returns a channel that is closed when the controller is finished.
	Done() <-chan struct{}
}
