package backend

import (
	"context"
)

// EsploraBackend implements Backend using the Esplora API (blockstream.info).
// The Esplora API is very similar to mempool.space, so we extend MempoolBackend.
type EsploraBackend struct {
	*MempoolBackend
}

// NewEsploraBackend creates a new Esplora backend.
func NewEsploraBackend(baseURL string) *EsploraBackend {
	return &EsploraBackend{
		MempoolBackend: NewMempoolBackend(baseURL),
	}
}

// Type returns TypeEsplora.
func (e *EsploraBackend) Type() Type {
	return TypeEsplora
}

// GetFeeEstimates returns fee estimates.
// Esplora uses a different endpoint than mempool.space
func (e *EsploraBackend) GetFeeEstimates(ctx context.Context) (*FeeEstimate, error) {
	// Esplora returns map of confirmation targets to fee rates
	var result map[string]float64
	if err := e.get(ctx, "/fee-estimates", &result); err != nil {
		return nil, err
	}

	// Esplora returns targets like "1", "2", "3", etc.
	// Map to our structure
	return &FeeEstimate{
		FastestFee:  uint64(result["1"]),   // 1 block
		HalfHourFee: uint64(result["3"]),   // 3 blocks (~30 min)
		HourFee:     uint64(result["6"]),   // 6 blocks (~1 hour)
		EconomyFee:  uint64(result["144"]), // 144 blocks (~1 day)
		MinimumFee:  1,                      // Esplora doesn't provide minimum
	}, nil
}

// Ensure EsploraBackend implements Backend
var _ Backend = (*EsploraBackend)(nil)
