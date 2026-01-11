package runoncedurationoverride

import (
	"context"

	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"

	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// Interface defines a controller.
type Interface interface {
	// Name returns the name of the controller.
	Name() string

	// WorkerCount returns the number of worker(s) that will process item(s)
	// off of the underlying work queue.s
	WorkerCount() int

	//Queue returns the underlying work queue associated with the controller.
	Queue() workqueue.RateLimitingInterface

	// Informer returns the underlying Informer object associated with the controller.
	Informer() cache.SharedIndexInformer

	// Reconciler returns the reconciler function that reconciles a request from a work queue.
	Reconciler() reconcile.Reconciler

	// Run starts the controller and blocks until the parent context is done.
	Run(parent context.Context, errorCh chan<- error)

	// Done returns a channel that is closed when the controller is finished.
	Done() <-chan struct{}
}
