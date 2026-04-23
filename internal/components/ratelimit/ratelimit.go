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

package ratelimit

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/TencentBlueKing/bk-bscp/internal/components"
	"github.com/TencentBlueKing/bk-bscp/internal/dal/bedis"
	"github.com/TencentBlueKing/bk-bscp/pkg/cc"
	"github.com/TencentBlueKing/bk-bscp/pkg/metrics"
)

var ErrRateLimited = errors.New("component rate limited")

const (
	componentRateLimitSubsystem = "component_rate_limit"
)

var (
	metricOnce   sync.Once
	globalMetric *limiterMetric
)

type limiterMetric struct {
	acquireTotal    *prometheus.CounterVec
	redisErrorTotal *prometheus.CounterVec
}

func initMetric() *limiterMetric {
	metricOnce.Do(func() {
		globalMetric = &limiterMetric{
			acquireTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
				Namespace: metrics.Namespace,
				Subsystem: componentRateLimitSubsystem,
				Name:      "acquire_total",
				Help:      "Total acquire attempts of component rate limiter",
			}, []string{"component", "result"}),
			redisErrorTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
				Namespace: metrics.Namespace,
				Subsystem: componentRateLimitSubsystem,
				Name:      "redis_error_total",
				Help:      "Total Redis errors of component rate limiter",
			}, []string{"component"}),
		}
		metrics.Register().MustRegister(globalMetric.acquireTotal)
		metrics.Register().MustRegister(globalMetric.redisErrorTotal)
	})
	return globalMetric
}

type fixedWindowLimiter struct {
	cfg    cc.ComponentRateLimit
	redis  bedis.Client
	now    func() time.Time
	metric *limiterMetric
}

func New(cfg cc.ComponentRateLimit, redisClient bedis.Client) (components.Limiter, error) {
	if !cfg.Enabled {
		return nil, nil
	}
	return NewRedisLimiter(cfg, redisClient)
}

func NewRedisLimiter(cfg cc.ComponentRateLimit, redisClient bedis.Client) (*fixedWindowLimiter, error) {
	if err := validateConfig(cfg); err != nil {
		return nil, err
	}
	if redisClient == nil {
		return nil, errors.New("redis client is required for component rate limit")
	}
	return &fixedWindowLimiter{
		cfg:    cfg,
		redis:  redisClient,
		now:    time.Now,
		metric: initMetric(),
	}, nil
}

func (l *fixedWindowLimiter) Acquire(ctx context.Context, component string) error {
	if l == nil || !l.cfg.Enabled {
		return nil
	}

	rule, ok := l.cfg.Components[component]
	if !ok || !rule.Enabled || rule.Limit == 0 {
		return nil
	}

	err := l.acquireRedis(ctx, component, rule)

	if err == nil {
		l.metric.acquireTotal.WithLabelValues(component, "allow").Inc()
		return nil
	}

	if errors.Is(err, ErrRateLimited) {
		l.metric.acquireTotal.WithLabelValues(component, "deny").Inc()
		return err
	}

	if l.cfg.FailOpenEnabled() {
		l.metric.acquireTotal.WithLabelValues(component, "fail_open").Inc()
		return nil
	}

	l.metric.acquireTotal.WithLabelValues(component, "error").Inc()
	return err
}

func (l *fixedWindowLimiter) acquireRedis(ctx context.Context, component string, rule cc.ComponentRateLimitRule) error {
	key := l.windowKey(component, l.now())

	count, err := l.redis.Incr(ctx, key)
	if err != nil {
		l.metric.redisErrorTotal.WithLabelValues(component).Inc()
		return fmt.Errorf("increment rate limit key %s failed: %w", key, err)
	}
	if count == 1 {
		if err := l.redis.Expire(ctx, key, int(l.cfg.KeyTTLSeconds), ""); err != nil {
			l.metric.redisErrorTotal.WithLabelValues(component).Inc()
			return fmt.Errorf("expire rate limit key %s failed: %w", key, err)
		}
	}
	if uint(count) > rule.Limit {
		return fmt.Errorf("%w: component=%s limit=%d window=%ds", ErrRateLimited, component, rule.Limit, l.cfg.WindowSeconds)
	}
	return nil
}

func (l *fixedWindowLimiter) windowKey(component string, now time.Time) string {
	windowStart := (now.Unix() / int64(l.cfg.WindowSeconds)) * int64(l.cfg.WindowSeconds)
	return fmt.Sprintf("bscp:component_ratelimit:%s:%d", component, windowStart)
}

func validateConfig(cfg cc.ComponentRateLimit) error {
	if !cfg.Enabled {
		return nil
	}
	if cfg.WindowSeconds == 0 {
		return errors.New("component rate limit windowSeconds must be greater than 0")
	}
	if cfg.KeyTTLSeconds == 0 {
		return errors.New("component rate limit keyTTLSeconds must be greater than 0")
	}
	if cfg.KeyTTLSeconds < cfg.WindowSeconds {
		return errors.New("component rate limit keyTTLSeconds must be greater than or equal to windowSeconds")
	}
	for component, rule := range cfg.Components {
		if !rule.Enabled {
			continue
		}
		if rule.Limit == 0 {
			return fmt.Errorf("component rate limit %s limit must be greater than 0", component)
		}
	}
	return nil
}
