// Copyright (c) 2016,2020 Uber Technologies, Inc.
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

package ratelimit // import "go.uber.org/ratelimit"

import (
	"time"

	"sync/atomic"
	"unsafe"
)

type state struct {
	last     time.Time
	sleepFor time.Duration
}

type atomicLimiter struct {
	state   unsafe.Pointer // 记录当前的限速状态，原子操作
	padding [56]byte       // padding 用于填充 CPU 缓存行（ cache line size - state pointer size = 64 - 8）; 防止伪共享缓存

	perRequest time.Duration // 每个请求的间隔
	maxSlack   time.Duration // 每个请求的最大松弛量
	clock      Clock         // Clock 计时器
}

// newAtomicBased 返回一个基于原子操作的限速器
func newAtomicBased(rate int, opts ...Option) *atomicLimiter {
	config := buildConfig(opts)
	l := &atomicLimiter{
		perRequest: config.per / time.Duration(rate),
		maxSlack:   -1 * config.maxSlack * time.Second / time.Duration(rate),
		clock:      config.clock,
	}

	// 初始化状态
	initialState := state{
		last:     time.Time{},
		sleepFor: 0,
	}
	atomic.StorePointer(&l.state, unsafe.Pointer(&initialState))
	return l
}

// Take 使用阻塞来保证多次 Take 调用的平均时间达到给定的 RPS
func (t *atomicLimiter) Take() time.Time {
	newState := state{}
	taken := false
	for !taken {
		now := t.clock.Now()

		previousStatePointer := atomic.LoadPointer(&t.state)
		oldState := (*state)(previousStatePointer)

		newState = state{}
		newState.last = now

		// 如果是首次调用，直接放行
		if oldState.last.IsZero() {
			taken = atomic.CompareAndSwapPointer(&t.state, previousStatePointer, unsafe.Pointer(&newState))
			continue
		}

		// sleepFor 通过 perRequest 和上次请求花费的时间来计算应该 sleep 多长时间
		// 由于请求的间隔可能会很长，skeepFor 可能为负数，在不同的请求之间累加
		newState.sleepFor += t.perRequest - now.Sub(oldState.last)

		// 我们不应该让 sleepFor 变得太负数
		// 因为这意味着在短时间内放慢很多速度的服务将在此之后获得更高的RPS。
		if newState.sleepFor < t.maxSlack {
			newState.sleepFor = t.maxSlack
		}

		// 如果 sleepFor > 0 说明无法抵消之前请求的时间，需要休眠一段时间
		if newState.sleepFor > 0 {
			newState.last = newState.last.Add(newState.sleepFor)
		}

		// 通过 for + cas 实现无锁化编程（lock free）
		taken = atomic.CompareAndSwapPointer(&t.state, previousStatePointer, unsafe.Pointer(&newState))
	}

	// sleep
	t.clock.Sleep(newState.sleepFor)
	return newState.last
}
