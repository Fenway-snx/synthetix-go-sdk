package synthetix

import (
	"context"
	"fmt"

	"github.com/synthetixio/synthetix-go/types"
)

// ExchangeStatus combines REST and WebSocket exchange status snapshots.
type ExchangeStatus struct {
	REST *types.ExchangeStatusResponse `json:"rest"`
	WS   *types.ExchangeStatusResponse `json:"ws"`
}

// IsRunning reports whether both public entrypoints are accepting orders.
func (s ExchangeStatus) IsRunning() bool {
	return s.REST != nil && s.WS != nil && s.REST.IsRunning() && s.WS.IsRunning()
}

// GetExchangeStatus checks the REST and WebSocket API status surfaces.
func (c *Client) GetExchangeStatus(ctx context.Context) (*ExchangeStatus, error) {
	restStatus, err := c.info.GetExchangeStatus(ctx)
	if err != nil {
		return nil, fmt.Errorf("synthetix: get REST exchange status: %w", err)
	}
	wsStatus, err := c.wsStatus.GetWSExchangeStatus(ctx)
	if err != nil {
		return nil, fmt.Errorf("synthetix: get WS exchange status: %w", err)
	}
	return &ExchangeStatus{
		REST: restStatus,
		WS:   wsStatus,
	}, nil
}
