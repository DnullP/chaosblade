/*
 * Copyright 2025 The ChaosBlade Authors
 *
		executors: exec.GetAllExecutors(),
	}
}
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
*/

package cri

import (
	"context"
	"fmt"

	"github.com/chaosblade-io/chaosblade-exec-cri/exec"
	"github.com/chaosblade-io/chaosblade-spec-go/channel"
	"github.com/chaosblade-io/chaosblade-spec-go/log"
	"github.com/chaosblade-io/chaosblade-spec-go/spec"
)

type Executor struct {
	executors map[string]spec.Executor
}

func NewExecutor() spec.Executor {
	return &Executor{
		executors: exec.GetAllExecutors(),
	}
}

func (*Executor) Name() string {
	return "cri"
}

func (e *Executor) Exec(uid string, ctx context.Context, model *spec.ExpModel) *spec.Response {
	// existing code
	key := exec.GetExecutorKey(model.Target, model.ActionName)
	executor := e.executors[key]
	if executor == nil {
		log.Errorf(ctx, "%s", spec.CriExecNotFound.Sprintf(key))
		return spec.ResponseFailWithFlags(spec.CriExecNotFound, key)
	}
	executor.SetChannel(channel.NewLocalChannel())
	return executor.Exec(uid, ctx, model)
}

func (*Executor) SetChannel(channel spec.Channel) {
}

// ExecInProcess executes a cri-targeted experiment by entering the target container's
// namespaces (via setns) and invoking the underlying executor in-process.
// PoC requirement: model.ActionFlags must contain "pid" (the target container main PID).
func (e *Executor) ExecInProcess(uid string, ctx context.Context, model *spec.ExpModel) *spec.Response {
	// resolve pid from flags
	pidStr := model.ActionFlags["pid"]
	if pidStr == "" {
		log.Errorf(ctx, "pid is required in action flags for cri ExecInProcess")
		return spec.ResponseFailWithFlags(spec.ParameterLess, "pid")
	}
	// parse pid
	var pid int
	if _, err := fmt.Sscanf(pidStr, "%d", &pid); err != nil {
		log.Errorf(ctx, "parse pid failed: %v", err)
		return spec.ResponseFailWithFlags(spec.ParameterIllegal, "pid", pidStr, err)
	}

	// select namespaces to enter
	namespaces := []string{"pid", "net", "mnt"}

	var resp *spec.Response

	// callback executed inside namespaces
	cb := func() error {
		key := exec.GetExecutorKey(model.Target, model.ActionName)
		executor := e.executors[key]
		if executor == nil {
			resp = spec.ResponseFailWithFlags(spec.CriExecNotFound, key)
			return fmt.Errorf("executor not found: %s", key)
		}
		// if underlying executor supports ExecInProcess, prefer that
		type inProc interface {
			ExecInProcess(string, context.Context, *spec.ExpModel) *spec.Response
		}
		if ip, ok := executor.(inProc); ok {
			resp = ip.ExecInProcess(uid, ctx, model)
		} else {
			// fallback: set local channel and call Exec
			executor.SetChannel(channel.NewLocalChannel())
			resp = executor.Exec(uid, ctx, model)
		}
		return nil
	}

	if err := runInNamespaces(pid, namespaces, cb); err != nil {
		log.Errorf(ctx, "enter namespaces failed: %v", err)
		return spec.ResponseFailWithFlags(spec.OsCmdExecFailed, err)
	}
	if resp == nil {
		return spec.ReturnFail(spec.OsCmdExecFailed, "no response from underlying executor")
	}
	return resp
}
