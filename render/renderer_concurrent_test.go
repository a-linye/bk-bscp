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

package render_test

import (
	"fmt"
	"sync"
	"testing"
)

// TestRenderer_ConcurrentReuse 验证多个 goroutine 并发调用同一个常驻渲染进程时，
// 请求与响应不会串扰，每个调用都拿到与自身上下文匹配的结果。
func TestRenderer_ConcurrentReuse(t *testing.T) {
	renderer := newTestRenderer(t)

	const goroutines = 20
	const iterations = 10

	var wg sync.WaitGroup
	errCh := make(chan error, goroutines*iterations)

	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				name := fmt.Sprintf("g%d-i%d", id, i)
				got, err := renderer.Render("Hello ${name}!", map[string]interface{}{"name": name})
				if err != nil {
					errCh <- fmt.Errorf("render failed for %s: %w", name, err)
					return
				}
				want := "Hello " + name + "!"
				if got != want {
					errCh <- fmt.Errorf("render mismatch: got %q, want %q", got, want)
					return
				}
			}
		}(g)
	}

	wg.Wait()
	close(errCh)

	for err := range errCh {
		t.Error(err)
	}
}
