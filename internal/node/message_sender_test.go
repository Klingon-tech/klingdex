package node

import (
	"testing"
	"time"
)

func TestDefaultMessageSenderConfig(t *testing.T) {
	cfg := DefaultMessageSenderConfig()

	// Verify defaults
	if cfg.InitialRetryInterval != 10*time.Second {
		t.Errorf("InitialRetryInterval = %v, want %v", cfg.InitialRetryInterval, 10*time.Second)
	}

	if cfg.MaxRetryInterval != 10*time.Minute {
		t.Errorf("MaxRetryInterval = %v, want %v", cfg.MaxRetryInterval, 10*time.Minute)
	}

	if cfg.BackoffMultiplier != 2.0 {
		t.Errorf("BackoffMultiplier = %v, want %v", cfg.BackoffMultiplier, 2.0)
	}

	if cfg.AckTimeout != 30*time.Second {
		t.Errorf("AckTimeout = %v, want %v", cfg.AckTimeout, 30*time.Second)
	}

	if cfg.StopBeforeExpiry != 1*time.Hour {
		t.Errorf("StopBeforeExpiry = %v, want %v", cfg.StopBeforeExpiry, 1*time.Hour)
	}

	if cfg.MaxRetries != 50 {
		t.Errorf("MaxRetries = %d, want %d", cfg.MaxRetries, 50)
	}
}

func TestMessageSenderConfigCustom(t *testing.T) {
	cfg := MessageSenderConfig{
		InitialRetryInterval: 5 * time.Second,
		MaxRetryInterval:     5 * time.Minute,
		BackoffMultiplier:    1.5,
		AckTimeout:           15 * time.Second,
		StopBeforeExpiry:     30 * time.Minute,
		MaxRetries:           20,
	}

	if cfg.InitialRetryInterval != 5*time.Second {
		t.Errorf("InitialRetryInterval = %v, want %v", cfg.InitialRetryInterval, 5*time.Second)
	}
	if cfg.MaxRetryInterval != 5*time.Minute {
		t.Errorf("MaxRetryInterval = %v, want %v", cfg.MaxRetryInterval, 5*time.Minute)
	}
	if cfg.BackoffMultiplier != 1.5 {
		t.Errorf("BackoffMultiplier = %v, want %v", cfg.BackoffMultiplier, 1.5)
	}
	if cfg.AckTimeout != 15*time.Second {
		t.Errorf("AckTimeout = %v, want %v", cfg.AckTimeout, 15*time.Second)
	}
	if cfg.StopBeforeExpiry != 30*time.Minute {
		t.Errorf("StopBeforeExpiry = %v, want %v", cfg.StopBeforeExpiry, 30*time.Minute)
	}
	if cfg.MaxRetries != 20 {
		t.Errorf("MaxRetries = %d, want %d", cfg.MaxRetries, 20)
	}
}

func TestBackoffCalculation(t *testing.T) {
	cfg := DefaultMessageSenderConfig()

	tests := []struct {
		retryCount int
		minBackoff time.Duration
		maxBackoff time.Duration
	}{
		{0, 10 * time.Second, 10 * time.Second},      // First retry: 10s
		{1, 20 * time.Second, 20 * time.Second},      // Second: 20s
		{2, 40 * time.Second, 40 * time.Second},      // Third: 40s
		{3, 80 * time.Second, 80 * time.Second},      // Fourth: 80s
		{4, 160 * time.Second, 160 * time.Second},    // Fifth: 160s
		{5, 320 * time.Second, 320 * time.Second},    // Sixth: 320s
		{6, 10 * time.Minute, 10 * time.Minute},      // Seventh: 640s -> capped at 600s (10min)
		{7, 10 * time.Minute, 10 * time.Minute},      // Eighth+: stays at max
		{10, 10 * time.Minute, 10 * time.Minute},     // Always capped
		{100, 10 * time.Minute, 10 * time.Minute},    // Always capped
	}

	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			backoff := calculateBackoff(cfg, tt.retryCount)
			if backoff < tt.minBackoff || backoff > tt.maxBackoff {
				t.Errorf("retry %d: backoff = %v, want between %v and %v",
					tt.retryCount, backoff, tt.minBackoff, tt.maxBackoff)
			}
		})
	}
}

// calculateBackoff mimics the backoff logic from MessageSender.scheduleRetry
func calculateBackoff(cfg MessageSenderConfig, retryCount int) time.Duration {
	backoff := cfg.InitialRetryInterval
	for i := 0; i < retryCount; i++ {
		backoff = time.Duration(float64(backoff) * cfg.BackoffMultiplier)
		if backoff > cfg.MaxRetryInterval {
			backoff = cfg.MaxRetryInterval
			break
		}
	}
	return backoff
}

func TestMaxRetriesEnforcement(t *testing.T) {
	cfg := DefaultMessageSenderConfig()

	// MaxRetries = 50 with exponential backoff to 10min
	// Initial ramp: 10+20+40+80+160+320 = 630s ≈ 10.5min
	// Remaining retries at max: 44 * 10min = 440min ≈ 7.3h
	// Total: approximately 7.5 hours

	// Calculate total retry time with default config
	totalTime := time.Duration(0)
	for i := 0; i < cfg.MaxRetries; i++ {
		totalTime += calculateBackoff(cfg, i)
	}

	// Should be at least 7 hours
	if totalTime < 7*time.Hour {
		t.Errorf("total retry time = %v, want at least 7h", totalTime)
	}

	// But not more than 9 hours
	if totalTime > 9*time.Hour {
		t.Errorf("total retry time = %v, should be less than 9h", totalTime)
	}
}

func TestSwapTimeoutCheck(t *testing.T) {
	cfg := DefaultMessageSenderConfig()

	// Swap expires in 2 hours
	swapTimeout := time.Now().Add(2 * time.Hour).Unix()

	// StopBeforeExpiry = 1 hour, so deadline is 1 hour from now
	deadline := time.Unix(swapTimeout, 0).Add(-cfg.StopBeforeExpiry)

	// Now should be before deadline
	if time.Now().After(deadline) {
		t.Error("deadline should be in the future")
	}

	// Deadline should be approximately 1 hour from now
	untilDeadline := time.Until(deadline)
	if untilDeadline < 50*time.Minute || untilDeadline > 70*time.Minute {
		t.Errorf("time until deadline = %v, want approximately 1h", untilDeadline)
	}
}

func TestSwapTimeoutExpired(t *testing.T) {
	cfg := DefaultMessageSenderConfig()

	// Swap expired 30 minutes ago
	swapTimeout := time.Now().Add(-30 * time.Minute).Unix()

	// Deadline would have been 1.5 hours ago
	deadline := time.Unix(swapTimeout, 0).Add(-cfg.StopBeforeExpiry)

	// Now should be after deadline
	if !time.Now().After(deadline) {
		t.Error("deadline should be in the past")
	}
}

func TestSwapTimeoutApproaching(t *testing.T) {
	cfg := DefaultMessageSenderConfig()

	// Swap expires in 30 minutes
	swapTimeout := time.Now().Add(30 * time.Minute).Unix()

	// With 1 hour buffer, deadline was 30 minutes ago
	deadline := time.Unix(swapTimeout, 0).Add(-cfg.StopBeforeExpiry)

	// Now should be after deadline (since we're within the buffer)
	if !time.Now().After(deadline) {
		t.Error("deadline should be in the past when swap timeout is approaching")
	}
}
