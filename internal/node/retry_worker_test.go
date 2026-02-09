package node

import (
	"testing"
	"time"
)

func TestDefaultRetryWorkerConfig(t *testing.T) {
	cfg := DefaultRetryWorkerConfig()

	// Verify defaults
	if cfg.PollInterval != 5*time.Second {
		t.Errorf("PollInterval = %v, want %v", cfg.PollInterval, 5*time.Second)
	}

	if cfg.CleanupInterval != 1*time.Hour {
		t.Errorf("CleanupInterval = %v, want %v", cfg.CleanupInterval, 1*time.Hour)
	}

	if cfg.BatchSize != 50 {
		t.Errorf("BatchSize = %d, want %d", cfg.BatchSize, 50)
	}

	if cfg.BufferDuration != 1*time.Hour {
		t.Errorf("BufferDuration = %v, want %v", cfg.BufferDuration, 1*time.Hour)
	}

	if cfg.RetentionPeriod != 7*24*time.Hour {
		t.Errorf("RetentionPeriod = %v, want %v", cfg.RetentionPeriod, 7*24*time.Hour)
	}
}

func TestRetryWorkerConfigCustom(t *testing.T) {
	cfg := RetryWorkerConfig{
		PollInterval:    10 * time.Second,
		CleanupInterval: 2 * time.Hour,
		BatchSize:       100,
		BufferDuration:  30 * time.Minute,
		RetentionPeriod: 14 * 24 * time.Hour,
	}

	if cfg.PollInterval != 10*time.Second {
		t.Errorf("PollInterval = %v, want %v", cfg.PollInterval, 10*time.Second)
	}
	if cfg.CleanupInterval != 2*time.Hour {
		t.Errorf("CleanupInterval = %v, want %v", cfg.CleanupInterval, 2*time.Hour)
	}
	if cfg.BatchSize != 100 {
		t.Errorf("BatchSize = %d, want %d", cfg.BatchSize, 100)
	}
	if cfg.BufferDuration != 30*time.Minute {
		t.Errorf("BufferDuration = %v, want %v", cfg.BufferDuration, 30*time.Minute)
	}
	if cfg.RetentionPeriod != 14*24*time.Hour {
		t.Errorf("RetentionPeriod = %v, want %v", cfg.RetentionPeriod, 14*24*time.Hour)
	}
}

func TestRetryWorkerBackoffCalculation(t *testing.T) {
	// The retry worker uses exponential backoff: 10s → 20s → 40s → 80s → 160s → 320s → 600s max
	tests := []struct {
		retryCount int
		minBackoff time.Duration
		maxBackoff time.Duration
	}{
		{0, 10 * time.Second, 10 * time.Second},
		{1, 20 * time.Second, 20 * time.Second},
		{2, 40 * time.Second, 40 * time.Second},
		{3, 80 * time.Second, 80 * time.Second},
		{4, 160 * time.Second, 160 * time.Second},
		{5, 320 * time.Second, 320 * time.Second},
		{6, 10 * time.Minute, 10 * time.Minute}, // Capped at max
		{7, 10 * time.Minute, 10 * time.Minute},
		{10, 10 * time.Minute, 10 * time.Minute},
	}

	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			backoff := simulateRetryWorkerBackoff(tt.retryCount)
			if backoff < tt.minBackoff || backoff > tt.maxBackoff {
				t.Errorf("retry %d: backoff = %v, want between %v and %v",
					tt.retryCount, backoff, tt.minBackoff, tt.maxBackoff)
			}
		})
	}
}

// simulateRetryWorkerBackoff mimics the backoff logic from RetryWorker.calculateNextRetry
func simulateRetryWorkerBackoff(retryCount int) time.Duration {
	baseInterval := 10 * time.Second
	maxInterval := 10 * time.Minute
	backoffMultiplier := 2.0

	backoff := baseInterval
	for i := 0; i < retryCount; i++ {
		backoff = time.Duration(float64(backoff) * backoffMultiplier)
		if backoff > maxInterval {
			backoff = maxInterval
			break
		}
	}
	return backoff
}

func TestRetentionPeriodSevenDays(t *testing.T) {
	cfg := DefaultRetryWorkerConfig()

	// 7 days in hours
	expectedHours := 7 * 24
	actualHours := int(cfg.RetentionPeriod.Hours())

	if actualHours != expectedHours {
		t.Errorf("RetentionPeriod = %d hours, want %d hours", actualHours, expectedHours)
	}
}

func TestCleanupIntervalOneHour(t *testing.T) {
	cfg := DefaultRetryWorkerConfig()

	if cfg.CleanupInterval.Hours() != 1 {
		t.Errorf("CleanupInterval = %v, want 1 hour", cfg.CleanupInterval)
	}
}

func TestBufferDurationBeforeSwapExpiry(t *testing.T) {
	cfg := DefaultRetryWorkerConfig()

	// Buffer is 1 hour before swap expiry - this is when we stop retrying
	if cfg.BufferDuration != 1*time.Hour {
		t.Errorf("BufferDuration = %v, want 1 hour", cfg.BufferDuration)
	}

	// Verify behavior: if swap expires at T, we stop retrying at T-1h
	swapTimeout := time.Now().Add(2 * time.Hour)
	stopRetryingAt := swapTimeout.Add(-cfg.BufferDuration)

	// stopRetryingAt should be 1 hour from now
	until := time.Until(stopRetryingAt)
	if until < 50*time.Minute || until > 70*time.Minute {
		t.Errorf("stop retrying in %v, want approximately 1 hour", until)
	}
}

func TestPollIntervalFiveSeconds(t *testing.T) {
	cfg := DefaultRetryWorkerConfig()

	if cfg.PollInterval != 5*time.Second {
		t.Errorf("PollInterval = %v, want 5 seconds", cfg.PollInterval)
	}
}

func TestBatchSizeFifty(t *testing.T) {
	cfg := DefaultRetryWorkerConfig()

	if cfg.BatchSize != 50 {
		t.Errorf("BatchSize = %d, want 50", cfg.BatchSize)
	}

	// Batch size should be reasonable - not too small, not too large
	if cfg.BatchSize < 10 || cfg.BatchSize > 1000 {
		t.Errorf("BatchSize = %d, should be between 10 and 1000", cfg.BatchSize)
	}
}
