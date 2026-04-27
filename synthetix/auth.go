package synthetix

import (
	"context"
)

// AuthStatus describes whether the client is ready for authenticated
// trading and what a caller should fix when it is not.
type AuthStatus struct {
	Mode                 AuthMode
	Ready                bool
	HasSigner            bool
	WalletAddress        string
	SubAccountID         uint64
	SubAccountDiscovered bool
	WSTradeReady         bool
	Issues               []string
}

// AuthStatus returns a local snapshot of auth configuration without
// making network calls.
func (c *Client) AuthStatus() AuthStatus {
	status := AuthStatus{Mode: c.AuthMode()}
	if c == nil {
		status.Issues = append(status.Issues, "client is nil")
		return status
	}
	status.HasSigner = c.signer != nil
	if status.HasSigner {
		status.WalletAddress = c.signer.WalletAddress()
	} else {
		status.Issues = append(status.Issues, "no signer configured; set Config.PrivateKeyHex or SYNTHETIX_PRIVATE_KEY")
	}
	if id, ok := c.DefaultSubAccountID(); ok {
		status.SubAccountID = id
		status.SubAccountDiscovered = true
	} else if status.HasSigner {
		status.Issues = append(status.Issues, "no default subaccount configured or discovered")
	}
	status.WSTradeReady = c.wsTrade != nil
	if status.HasSigner && status.SubAccountID != 0 && !status.WSTradeReady {
		status.Issues = append(status.Issues, "authenticated websocket is not initialized; call SetDefaultSubAccountID or ValidateAuth")
	}
	status.Ready = status.HasSigner && status.SubAccountID != 0 && len(status.Issues) == 0
	return status
}

// ValidateAuth verifies signer and subaccount readiness, performing
// network-based subaccount discovery when needed.
func (c *Client) ValidateAuth(ctx context.Context) (*AuthStatus, error) {
	if c == nil {
		status := AuthStatus{Mode: AuthModeReadOnly, Issues: []string{"client is nil"}}
		return &status, nil
	}
	if c.signer != nil {
		if _, ok := c.DefaultSubAccountID(); !ok {
			if _, err := c.DiscoverDefaultSubAccount(ctx); err != nil {
				status := c.AuthStatus()
				status.Issues = append(status.Issues, err.Error())
				status.Ready = false
				return &status, err
			}
		}
		if id, ok := c.DefaultSubAccountID(); ok {
			if err := c.ensureWSTrade(id); err != nil {
				status := c.AuthStatus()
				status.Issues = append(status.Issues, err.Error())
				status.Ready = false
				return &status, err
			}
		}
	}
	status := c.AuthStatus()
	status.Ready = status.HasSigner && status.SubAccountID != 0
	if status.Ready {
		status.Issues = nil
	}
	return &status, nil
}

