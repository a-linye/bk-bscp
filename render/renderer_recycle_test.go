/*
 * Tencent is pleased to support the open source community by making Blueking Container Service available.
 * Copyright (C) 2019 THL A29 Limited, a Tencent company. All rights reserved.
 * Licensed under the MIT License (the "License"); you may not use this file except
 * in compliance with the License. You may obtain a copy of the License at
 * http://opensource.org/licenses/MIT
 * Unless required by applicable law or agreed to in writing, software distributed under
 * the License is distributed on an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND,
 * either express or implied. See the License for the specific language governing permissions and
 * limitations under the License.
 */

package render

import (
	"os"
	"os/exec"
	"testing"
	"time"
)

// startIdleWorker 用一个空转进程模拟 worker，避免测试依赖 uv/python 环境
func startIdleWorker(t *testing.T) *renderWorker {
	t.Helper()

	cmd := exec.Command("sleep", "60")
	if err := cmd.Start(); err != nil {
		t.Skipf("cannot start helper process: %v", err)
	}
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	})
	return &renderWorker{cmd: cmd}
}

func TestWorkerRSSMBReadsCurrentProcess(t *testing.T) {
	rssMB, ok := workerRSSMB(os.Getpid())
	if !ok {
		t.Skip("process RSS is unavailable on this platform")
	}
	if rssMB <= 0 {
		t.Fatalf("workerRSSMB = %d, want > 0", rssMB)
	}
}

// 线上 worker 是 uv 壳进程 fork 出 python3，内存都在子进程上，
// 只统计壳进程会让回收永远不触发
func TestWorkerRSSMBIncludesChildProcesses(t *testing.T) {
	cmd := exec.Command("sh", "-c", "sleep 60 & wait")
	if err := cmd.Start(); err != nil {
		t.Skipf("cannot start helper process: %v", err)
	}
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	})

	pid := cmd.Process.Pid
	var children []int
	for i := 0; i < 50; i++ {
		if children = childPIDs(pid); len(children) > 0 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if len(children) == 0 {
		t.Skip("child process is not observable on this platform")
	}

	selfKB, ok := processRSSKB(pid)
	if !ok {
		t.Skip("process RSS is unavailable on this platform")
	}
	wantKB := selfKB
	for _, child := range children {
		childKB, childOK := processRSSKB(child)
		if !childOK {
			t.Skip("child RSS is unavailable on this platform")
		}
		wantKB += childKB
	}

	got, ok := workerRSSMB(pid)
	if !ok {
		t.Fatal("workerRSSMB returned ok=false")
	}
	if got != wantKB/1024 {
		t.Fatalf("workerRSSMB = %d MB, want %d MB (self+children)", got, wantKB/1024)
	}
}

func TestRecycleIfNeededOnRSSLimit(t *testing.T) {
	w := startIdleWorker(t)
	rssMB, ok := workerRSSMB(w.cmd.Process.Pid)
	if !ok {
		t.Skip("process RSS is unavailable on this platform")
	}

	// 上限取进程当前占用，必然触发重建
	r := &Renderer{workerRSSLimitMB: rssMB, workerMaxUses: 1000}
	r.recycleIfNeeded(w)

	if !w.dead {
		t.Fatalf("worker not recycled at rss limit %dMB", rssMB)
	}
}

func TestRecycleIfNeededKeepsWorkerBelowLimits(t *testing.T) {
	w := startIdleWorker(t)
	rssMB, ok := workerRSSMB(w.cmd.Process.Pid)
	if !ok {
		t.Skip("process RSS is unavailable on this platform")
	}

	r := &Renderer{workerRSSLimitMB: rssMB + 1024, workerMaxUses: 1000}
	r.recycleIfNeeded(w)

	if w.dead {
		t.Fatal("worker recycled while below rss limit")
	}
	if w.uses != 1 {
		t.Fatalf("uses = %d, want 1", w.uses)
	}
}

// 读不到进程常驻内存时（非 Linux 等），应退化为按渲染次数重建
func TestRecycleIfNeededFallsBackToUses(t *testing.T) {
	cmd := exec.Command("true")
	if err := cmd.Start(); err != nil {
		t.Skipf("cannot start helper process: %v", err)
	}
	_ = cmd.Wait()
	// 进程已退出，/proc 中不再有该 pid，等价于读不到 RSS
	if _, ok := workerRSSMB(cmd.Process.Pid); ok {
		t.Skip("pid still readable, cannot simulate missing RSS")
	}

	w := &renderWorker{cmd: cmd, uses: 4}
	r := &Renderer{workerRSSLimitMB: defaultWorkerRSSLimitMB, workerMaxUses: 5}
	r.recycleIfNeeded(w)

	if !w.dead {
		t.Fatal("worker not recycled after reaching max uses")
	}
}
