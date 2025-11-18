/*
 * Copyright 2025 The ChaosBlade Authors
 *
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

package os

import (
    "context"
    "testing"

    "github.com/chaosblade-io/chaosblade-spec-go/spec"
)

func TestExecInProcessCpuStartAndDestroy(t *testing.T) {
    executorIf := NewExecutor()
    model := &spec.ExpModel{Target: "os", ActionName: "load", ActionFlags: map[string]string{"cpu-percent": "10"}}
    uid := "test-uid-1"
    real, ok := executorIf.(*Executor)
    if !ok {
        t.Fatalf("executor type assertion failed")
    }
    resp := real.ExecInProcess(uid, context.Background(), model)
    if !resp.Success {
        t.Fatalf("expected success start, got err: %v", resp.Err)
    }

    // destroy
    ctx := spec.SetDestroyFlag(context.Background(), uid)
    resp2 := real.ExecInProcess(uid, ctx, model)
    if !resp2.Success {
        t.Fatalf("expected success destroy, got err: %v", resp2.Err)
    }
}
