package dispatcher

import (
	"context"
	"fmt"
	"sync"

	"github.com/chaosblade-io/chaosblade-spec-go/spec"
)

// ExecutionRequest describes the information needed to route a request to an executor.
type ExecutionRequest struct {
	Scope  string
	Target string
	Action string
	UID    string
	Model  *spec.ExpModel
	// Destroy indicates whether the request is a destroy operation.
	Destroy bool
}

// Dispatcher maintains a registry of executors and routes experiment requests.
type Dispatcher struct {
	mu        sync.RWMutex
	executors map[string]spec.Executor
}

// New creates an empty dispatcher instance.
func New() *Dispatcher {
	return &Dispatcher{
		executors: make(map[string]spec.Executor),
	}
}

// Register binds an executor to a target/action pair. Scope is used for nested targets such as docker or cri.
func (d *Dispatcher) Register(scope, target, action string, executor spec.Executor) {
	if executor == nil {
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	key := createExecutorKey(scope, target, action)
	d.executors[key] = executor
}

// Get returns the executor registered for the provided scope/target/action triplet.
func (d *Dispatcher) Get(scope, target, action string) spec.Executor {
	key := createExecutorKey(scope, target, action)
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.executors[key]
}

// Dispatch locates the executor and invokes it with the provided model.
func (d *Dispatcher) Dispatch(ctx context.Context, req ExecutionRequest) (*spec.Response, error) {
	executor := d.Get(req.Scope, req.Target, req.Action)
	if executor == nil {
		return nil, spec.ResponseFailWithFlags(spec.HandlerExecNotFound,
			fmt.Sprintf("scope=%s target=%s action=%s", req.Scope, req.Target, req.Action))
	}

	// mark destroy requests in the context so downstream executors behave consistently with the CLI.
	if req.Destroy {
		ctx = spec.SetDestroyFlag(ctx, req.UID)
	}
	return executor.Exec(req.UID, ctx, req.Model), nil
}

func createExecutorKey(scope, target, action string) string {
	key := scope
	parts := []string{target, action}
	for _, item := range parts {
		if item == "" {
			continue
		}
		if key == "" {
			key = item
		} else {
			key = fmt.Sprintf("%s-%s", key, item)
		}
	}
	return key
}
