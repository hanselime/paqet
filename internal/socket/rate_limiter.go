package socket

import (
	"context"
	"time"

	"golang.org/x/time/rate"
)

type RateLimiter struct {
	limiter  *rate.Limiter
	enabled  bool
	adaptive bool
}

type RateLimiterConfig struct {
	Enabled          bool
	PacketsPerSecond int
	Burst            int
	Adaptive         bool
}


func NewRateLimiter(config RateLimiterConfig) *RateLimiter {
	if !config.Enabled || config.PacketsPerSecond <= 0 {
		return &RateLimiter{enabled: false}
	}

	return &RateLimiter{
		limiter:  rate.NewLimiter(rate.Limit(config.PacketsPerSecond), config.Burst),
		enabled:  true,
		adaptive: config.Adaptive,
	}
}


func (rl *RateLimiter) Wait(ctx context.Context) error {
	if !rl.enabled {
		return nil
	}
	return rl.limiter.Wait(ctx)
}


func (rl *RateLimiter) Allow() bool {
	if !rl.enabled {
		return true
	}
	return rl.limiter.Allow()
}


func (rl *RateLimiter) Reserve() *rate.Reservation {
	if !rl.enabled {
		return nil
	}
	return rl.limiter.Reserve()
}


func (rl *RateLimiter) SetRate(packetsPerSecond int, burst int) {
	if !rl.enabled {
		return
	}
	
	rl.limiter.SetLimit(rate.Limit(packetsPerSecond))
	rl.limiter.SetBurst(burst)
}


func (rl *RateLimiter) GetRate() (packetsPerSecond float64, burst int) {
	if !rl.enabled {
		return 0, 0
	}
	
	return float64(rl.limiter.Limit()), rl.limiter.Burst()
}


func (rl *RateLimiter) IsEnabled() bool {
	return rl.enabled
}

func (rl *RateLimiter) AdaptiveAdjust(packetLossPercent float64) {
	if !rl.enabled || !rl.adaptive {
		return
	}

	currentRate, currentBurst := rl.GetRate()
	
	if packetLossPercent > 10.0 {
		newRate := currentRate * 0.9
		rl.SetRate(int(newRate), currentBurst)
	} else if packetLossPercent < 1.0 && currentRate < 10000 {
		newRate := currentRate * 1.05
		rl.SetRate(int(newRate), currentBurst)
	}
}


func (rl *RateLimiter) WaitWithTimeout(timeout time.Duration) error {
	if !rl.enabled {
		return nil
	}
	
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	
	return rl.Wait(ctx)
}
