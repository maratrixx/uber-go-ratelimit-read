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

	"github.com/andres-erbsen/clock"
)

// 注意：灵感来自于:
// https://github.com/prashantv/go-bench/blob/master/ratelimit

// Limiter 是用于限流的接口，可能会跨协程
// 在每次执行之前需要先调用 Take()，这可能会阻塞 goroutine
type Limiter interface {
	// Take 可能会发生阻塞以满足每秒允许请求量（RPS）
	Take() time.Time
}

// Clock 是实现限速器的最小必要接口，与 github.com/andres-erbsen/clock 保持兼容
type Clock interface {
	Now() time.Time
	Sleep(time.Duration)
}

// Limiter 配置项
type config struct {
	clock    Clock         // Clock 接口
	maxSlack time.Duration // 最大松弛量
	per      time.Duration // 限速时间窗口，默认是 1 秒
}

// New 以给定的速率和可选项生成 Limiter 限速器
func New(rate int, opts ...Option) Limiter {
	return newAtomicBased(rate, opts...)
}

// buildConfig 合并默认配置和自定义配置
func buildConfig(opts []Option) config {
	c := config{
		clock:    clock.New(),
		maxSlack: 10,
		per:      time.Second,
	}

	for _, opt := range opts {
		opt.apply(&c)
	}
	return c
}

// Option 接口
type Option interface {
	apply(*config)
}

type clockOption struct {
	clock Clock
}

func (o clockOption) apply(c *config) {
	c.clock = o.clock
}

// WithClock 使用自定义的 Clock 接口实现，主要用于测试
func WithClock(clock Clock) Option {
	return clockOption{clock: clock}
}

type slackOption int

func (o slackOption) apply(c *config) {
	c.maxSlack = time.Duration(o)
}

// WithoutSlack 不容忍任何突发流量配置
var WithoutSlack Option = slackOption(0)

type perOption time.Duration

func (p perOption) apply(c *config) {
	c.per = time.Duration(p)
}

// Per 配置不同时间窗口的限速，默认的时间窗口为 1 秒
// New(100)即为每秒 100 个请求
// New(2, Per(60*time.Second))即为每分钟 2 个请求
func Per(per time.Duration) Option {
	return perOption(per)
}

type unlimited struct{}

// NewUnlimited 对请求不作任何的限速
func NewUnlimited() Limiter {
	return unlimited{}
}

func (unlimited) Take() time.Time {
	return time.Now()
}
