package types

// SignedEnvelope is the generic /v1/trade signed-request shape:
//
//	{
//	  "subaccountId":  "<uint64>",
//	  "walletAddress": "0x...",
//	  "signature":     {"v": ..., "r": "0x...", "s": "0x..."},
//	  "nonce":         <uint64>,
//	  "expiresAfter":  <unix-seconds>,
//	  "params":        { "action": "<name>", ... }
//	}
//
// All Signer.Sign<Action>() methods return either a typed wrapper from
// trade_types.go (e.g. *PlaceOrdersRequest) or this escape-hatch shape
// for new endpoints that have not yet graduated to a dedicated type.
// The API treats params.action as authoritative; Signer always
// embeds the right action string so the posted body matches the bytes
// that were signed.
type SignedEnvelope struct {
	Params        map[string]any      `json:"params"`
	Signature     SignatureComponents `json:"signature"`
	SubAccountID  string              `json:"subaccountId"`
	WalletAddress string              `json:"walletAddress"`
	Nonce         uint64              `json:"nonce"`
	ExpiresAfter  int64               `json:"expiresAfter,omitempty"`
}
