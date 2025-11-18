package os

import (
	"context"
	"fmt"
	"runtime"
	"sync"
	"time"

	"github.com/chaosblade-io/chaosblade-spec-go/log"
	"github.com/chaosblade-io/chaosblade-spec-go/spec"
)

var (
	workers = make(map[string]chan struct{})
	wmu     sync.Mutex
)

// ExecInProcess provides a minimal in-process implementation for OS actions (PoC)
func (e *Executor) ExecInProcess(uid string, ctx context.Context, model *spec.ExpModel) *spec.Response {
	_, isDestroy := spec.IsDestroy(ctx)

	// simple handler: CPU load when flag cpu-percent provided
	cpuPercent := model.ActionFlags["cpu-percent"]
	if cpuPercent == "" {
		return spec.ReturnFail(spec.ParameterIllegal, "cpu-percent flag is required for inprocess PoC")
	}

	if isDestroy {
		// stop worker
		wmu.Lock()
		ch, ok := workers[uid]
		if ok {
			close(ch)
			delete(workers, uid)
		}
		wmu.Unlock()
		log.Infof(ctx, "destroyed in-process worker for uid %s", uid)
		return spec.ReturnSuccess("destroyed")
	}

	// start worker
	percent := 0
	if _, err := fmt.Sscanf(cpuPercent, "%d", &percent); err != nil {
		return spec.ReturnFail(spec.ParameterIllegal, "cpu-percent must be integer")
	}
	if percent <= 0 || percent > 100 {
		return spec.ReturnFail(spec.ParameterIllegal, "cpu-percent must be between 1 and 100")
	}

	wmu.Lock()
	if _, exists := workers[uid]; exists {
		wmu.Unlock()
		return spec.ReturnFail(spec.ParameterIllegal, "uid already exists")
	}
	stop := make(chan struct{})
	workers[uid] = stop
	wmu.Unlock()

	// start a background goroutine that attempts to consume CPU
	go func() {
		// keep this goroutine on its own thread to avoid affecting scheduler decisions
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		log.Infof(ctx, "start cpu worker uid %s percent %d", uid, percent)
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()
		busyMs := int(100 * percent / 100) // for PoC: busy for percent% of 100ms window
		idleMs := 100 - busyMs
		for {
			select {
			case <-stop:
				log.Infof(ctx, "stop cpu worker uid %s", uid)
				return
			default:
				// busy loop
				end := time.Now().Add(time.Duration(busyMs) * time.Millisecond)
				for time.Now().Before(end) {
				}
				if idleMs > 0 {
					select {
					case <-stop:
						return
					case <-time.After(time.Duration(idleMs) * time.Millisecond):
					}
				}
			}
		}
	}()

	return spec.ReturnSuccess(uid)
}
