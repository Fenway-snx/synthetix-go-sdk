package signer

import (
	"time"

	"github.com/Fenway-snx/synthetix-go-sdk/eip712"
	"github.com/Fenway-snx/synthetix-go-sdk/types"
)

// PlaceOrderInput is the unsigned input shape for one order inside
// a SignPlaceOrders call. The Signer fans the value out to both the
// EIP-712 builder (for the digest) and the wire payload (for the
// POST body). Fields that are not in the EIP-712 order schema are
// still included in the wire payload when set.
type PlaceOrderInput struct {
	Symbol          string
	Side            string
	OrderType       string
	Price           string
	TriggerPrice    string
	Quantity        string
	ReduceOnly      bool
	PostOnly        bool
	IsTriggerMarket bool
	ClientOrderID   string
	ClosePosition   bool
	ExpiresAt       int64
	TimeInForce     string
	DurationSeconds int64
	IntervalSeconds int64
}

// SignPlaceOrders signs a placeOrders action and returns the
// fully-formed envelope ready to POST via
// resttrade.Client.PlaceOrders. Pass 0 for nonce/expiresAfterMs to
// have the Signer pick safe defaults (NextNonce + DefaultExpiresAfter).
func (s *Signer) SignPlaceOrders(
	subAccountID uint64,
	orders []PlaceOrderInput,
	grouping string,
	nonce uint64,
	expiresAfter int64,
) (*types.PlaceOrdersRequest, error) {
	if nonce == 0 {
		nonce = s.NextNonce()
	}
	expiresAfter = resolveExpiry(nonce, expiresAfter)

	eip712Orders := make([]eip712.PlaceOrderItem, len(orders))
	wireOrders := make([]types.PlaceOrderItem, len(orders))
	for i, o := range orders {
		eip712Orders[i] = eip712.PlaceOrderItem{
			Symbol:          o.Symbol,
			Side:            o.Side,
			OrderType:       o.OrderType,
			Price:           o.Price,
			TriggerPrice:    o.TriggerPrice,
			Quantity:        o.Quantity,
			ReduceOnly:      o.ReduceOnly,
			IsTriggerMarket: o.IsTriggerMarket,
			ClientOrderID:   o.ClientOrderID,
			ClosePosition:   o.ClosePosition,
		}
		wireOrders[i] = types.PlaceOrderItem{
			Symbol:          o.Symbol,
			Side:            o.Side,
			OrderType:       o.OrderType,
			Price:           o.Price,
			TriggerPrice:    o.TriggerPrice,
			Quantity:        o.Quantity,
			ReduceOnly:      o.ReduceOnly,
			PostOnly:        o.PostOnly,
			IsTriggerMarket: o.IsTriggerMarket,
			ClientOrderID:   o.ClientOrderID,
			ClosePosition:   o.ClosePosition,
			ExpiresAt:       o.ExpiresAt,
			TimeInForce:     o.TimeInForce,
			DurationSeconds: o.DurationSeconds,
			IntervalSeconds: o.IntervalSeconds,
		}
	}

	typedData := eip712.BuildPlaceOrders(subAccountID, eip712Orders, grouping, nonce, expiresAfter)
	sig, err := s.signTypedData(typedData)
	if err != nil {
		return nil, err
	}

	return &types.PlaceOrdersRequest{
		Params: map[string]any{
			"action":   "placeOrders",
			"orders":   wireOrders,
			"grouping": grouping,
		},
		Signature:     sig,
		SubAccountID:  uintStr(subAccountID),
		WalletAddress: s.WalletAddress(),
		Nonce:         nonce,
		ExpiresAfter:  expiresAfter,
	}, nil
}

// SignCancelOrders signs a cancelOrders action by venue order id.
func (s *Signer) SignCancelOrders(
	subAccountID uint64,
	orderIDs []uint64,
	nonce uint64,
	expiresAfter int64,
) (*types.CancelOrdersRequest, error) {
	if nonce == 0 {
		nonce = s.NextNonce()
	}
	expiresAfter = resolveExpiry(nonce, expiresAfter)

	typedData := eip712.BuildCancelOrders(subAccountID, orderIDs, nonce, expiresAfter)
	sig, err := s.signTypedData(typedData)
	if err != nil {
		return nil, err
	}

	wireIDs := make([]string, len(orderIDs))
	for i, id := range orderIDs {
		wireIDs[i] = uintStr(id)
	}

	return &types.CancelOrdersRequest{
		Params: map[string]any{
			"action":   "cancelOrders",
			"orderIds": wireIDs,
		},
		Signature:     sig,
		SubAccountID:  uintStr(subAccountID),
		WalletAddress: s.WalletAddress(),
		Nonce:         nonce,
		ExpiresAfter:  expiresAfter,
	}, nil
}

// SignCancelOrdersByCloid signs a cancelOrders action by client
// order id.
func (s *Signer) SignCancelOrdersByCloid(
	subAccountID uint64,
	clientOrderIDs []string,
	nonce uint64,
	expiresAfter int64,
) (*types.CancelOrdersRequest, error) {
	if nonce == 0 {
		nonce = s.NextNonce()
	}
	expiresAfter = resolveExpiry(nonce, expiresAfter)

	typedData := eip712.BuildCancelOrdersByCloid(subAccountID, clientOrderIDs, nonce, expiresAfter)
	sig, err := s.signTypedData(typedData)
	if err != nil {
		return nil, err
	}

	return &types.CancelOrdersRequest{
		Params: map[string]any{
			"action":         "cancelOrders",
			"clientOrderIds": clientOrderIDs,
		},
		Signature:     sig,
		SubAccountID:  uintStr(subAccountID),
		WalletAddress: s.WalletAddress(),
		Nonce:         nonce,
		ExpiresAfter:  expiresAfter,
	}, nil
}

// SignCancelAllOrders signs a cancelAllOrders action. symbols may
// be nil/empty for "cancel everything".
func (s *Signer) SignCancelAllOrders(
	subAccountID uint64,
	symbols []string,
	nonce uint64,
	expiresAfter int64,
) (*types.CancelAllOrdersRequest, error) {
	if nonce == 0 {
		nonce = s.NextNonce()
	}
	expiresAfter = resolveExpiry(nonce, expiresAfter)

	typedData := eip712.BuildCancelAllOrders(subAccountID, symbols, nonce, expiresAfter)
	sig, err := s.signTypedData(typedData)
	if err != nil {
		return nil, err
	}

	if symbols == nil {
		symbols = []string{}
	}

	return &types.CancelAllOrdersRequest{
		Params: map[string]any{
			"action":  "cancelAllOrders",
			"symbols": symbols,
		},
		Signature:     sig,
		SubAccountID:  uintStr(subAccountID),
		WalletAddress: s.WalletAddress(),
		Nonce:         nonce,
		ExpiresAfter:  expiresAfter,
	}, nil
}

// SignModifyOrder signs a modifyOrder action by venue order id.
// Pass empty string for any field that is not being modified.
func (s *Signer) SignModifyOrder(
	subAccountID, orderID uint64,
	price, quantity, triggerPrice string,
	nonce uint64,
	expiresAfter int64,
) (*types.ModifyOrderRequest, error) {
	if nonce == 0 {
		nonce = s.NextNonce()
	}
	expiresAfter = resolveExpiry(nonce, expiresAfter)

	typedData := eip712.BuildModifyOrder(subAccountID, orderID, price, quantity, triggerPrice, nonce, expiresAfter)
	sig, err := s.signTypedData(typedData)
	if err != nil {
		return nil, err
	}

	params := map[string]any{
		"action":  "modifyOrder",
		"orderId": uintStr(orderID),
	}
	if price != "" {
		params["price"] = price
	}
	if quantity != "" {
		params["quantity"] = quantity
	}
	if triggerPrice != "" {
		params["triggerPrice"] = triggerPrice
	}

	return &types.ModifyOrderRequest{
		Params:        params,
		Signature:     sig,
		SubAccountID:  uintStr(subAccountID),
		WalletAddress: s.WalletAddress(),
		Nonce:         nonce,
		ExpiresAfter:  expiresAfter,
	}, nil
}

// SignModifyOrderByCloid signs a modifyOrder action by client order
// id.
func (s *Signer) SignModifyOrderByCloid(
	subAccountID uint64,
	clientOrderID, price, quantity, triggerPrice string,
	nonce uint64,
	expiresAfter int64,
) (*types.ModifyOrderRequest, error) {
	if nonce == 0 {
		nonce = s.NextNonce()
	}
	expiresAfter = resolveExpiry(nonce, expiresAfter)

	typedData := eip712.BuildModifyOrderByCloid(subAccountID, clientOrderID, price, quantity, triggerPrice, nonce, expiresAfter)
	sig, err := s.signTypedData(typedData)
	if err != nil {
		return nil, err
	}

	params := map[string]any{
		"action":        "modifyOrder",
		"clientOrderId": clientOrderID,
	}
	if price != "" {
		params["price"] = price
	}
	if quantity != "" {
		params["quantity"] = quantity
	}
	if triggerPrice != "" {
		params["triggerPrice"] = triggerPrice
	}

	return &types.ModifyOrderRequest{
		Params:        params,
		Signature:     sig,
		SubAccountID:  uintStr(subAccountID),
		WalletAddress: s.WalletAddress(),
		Nonce:         nonce,
		ExpiresAfter:  expiresAfter,
	}, nil
}

// SignUpdateLeverage signs an updateLeverage action. leverage is a
// decimal string multiplier (e.g. "5", "10.5").
func (s *Signer) SignUpdateLeverage(
	subAccountID uint64,
	symbol, leverage string,
	nonce uint64,
	expiresAfter int64,
) (*types.UpdateLeverageRequest, error) {
	if nonce == 0 {
		nonce = s.NextNonce()
	}
	expiresAfter = resolveExpiry(nonce, expiresAfter)

	typedData := eip712.BuildUpdateLeverage(subAccountID, symbol, leverage, nonce, expiresAfter)
	sig, err := s.signTypedData(typedData)
	if err != nil {
		return nil, err
	}

	return &types.UpdateLeverageRequest{
		Params: map[string]any{
			"action":   "updateLeverage",
			"symbol":   symbol,
			"leverage": leverage,
		},
		Signature:     sig,
		SubAccountID:  uintStr(subAccountID),
		WalletAddress: s.WalletAddress(),
		Nonce:         nonce,
		ExpiresAfter:  expiresAfter,
	}, nil
}

// SignWithdrawCollateral signs a withdrawCollateral action.
// destination is a 0x-prefixed hex address.
func (s *Signer) SignWithdrawCollateral(
	subAccountID uint64,
	symbol, amount, destination string,
	nonce uint64,
	expiresAfter int64,
) (*types.WithdrawCollateralRequest, error) {
	if nonce == 0 {
		nonce = s.NextNonce()
	}
	expiresAfter = resolveExpiry(nonce, expiresAfter)

	typedData := eip712.BuildWithdrawCollateral(subAccountID, symbol, amount, destination, nonce, expiresAfter)
	sig, err := s.signTypedData(typedData)
	if err != nil {
		return nil, err
	}

	return &types.WithdrawCollateralRequest{
		Params: map[string]any{
			"action":      "withdrawCollateral",
			"symbol":      symbol,
			"amount":      amount,
			"destination": destination,
		},
		Signature:     sig,
		SubAccountID:  uintStr(subAccountID),
		WalletAddress: s.WalletAddress(),
		Nonce:         nonce,
		ExpiresAfter:  expiresAfter,
	}, nil
}

// SignTransferCollateral signs a transferCollateral action between
// two subaccounts on the same wallet.
func (s *Signer) SignTransferCollateral(
	fromSubAccountID, toSubAccountID uint64,
	symbol, amount string,
	nonce uint64,
	expiresAfter int64,
) (*types.TransferCollateralRequest, error) {
	if nonce == 0 {
		nonce = s.NextNonce()
	}
	expiresAfter = resolveExpiry(nonce, expiresAfter)

	typedData := eip712.BuildTransferCollateral(fromSubAccountID, toSubAccountID, symbol, amount, nonce, expiresAfter)
	sig, err := s.signTypedData(typedData)
	if err != nil {
		return nil, err
	}

	return &types.TransferCollateralRequest{
		Params: map[string]any{
			"action": "transferCollateral",
			"symbol": symbol,
			"amount": amount,
			"to":     uintStr(toSubAccountID),
		},
		Signature:     sig,
		SubAccountID:  uintStr(fromSubAccountID),
		WalletAddress: s.WalletAddress(),
		Nonce:         nonce,
		ExpiresAfter:  expiresAfter,
	}, nil
}

// SignCreateSubaccount signs a createSubaccount action. masterSubAccountID
// is the existing subaccount whose ownership the caller is proving;
// the new subaccount's id is assigned by the API.
func (s *Signer) SignCreateSubaccount(
	masterSubAccountID uint64,
	name string,
	nonce uint64,
	expiresAfter int64,
) (*types.CreateSubaccountRequest, error) {
	if nonce == 0 {
		nonce = s.NextNonce()
	}
	expiresAfter = resolveExpiry(nonce, expiresAfter)

	typedData := eip712.BuildCreateSubaccount(masterSubAccountID, name, nonce, expiresAfter)
	sig, err := s.signTypedData(typedData)
	if err != nil {
		return nil, err
	}

	return &types.CreateSubaccountRequest{
		Params: types.CreateSubaccountAction{
			Action:       "createSubaccount",
			SubAccountID: uintStr(masterSubAccountID),
			Name:         name,
		},
		Nonce:        nonce,
		Signature:    sig,
		ExpiresAfter: expiresAfter,
	}, nil
}

// SignUpdateSubAccountName signs an updateSubAccountName action.
func (s *Signer) SignUpdateSubAccountName(
	subAccountID uint64,
	name string,
	nonce uint64,
	expiresAfter int64,
) (*types.UpdateSubAccountNameRequest, error) {
	if nonce == 0 {
		nonce = s.NextNonce()
	}
	expiresAfter = resolveExpiry(nonce, expiresAfter)

	typedData := eip712.BuildUpdateSubAccountName(subAccountID, name, nonce, expiresAfter)
	sig, err := s.signTypedData(typedData)
	if err != nil {
		return nil, err
	}

	return &types.UpdateSubAccountNameRequest{
		Params: types.UpdateSubAccountNameAction{
			Action: "updateSubAccountName",
			Name:   name,
		},
		SubAccountID: uintStr(subAccountID),
		Nonce:        nonce,
		Signature:    sig,
		ExpiresAfter: expiresAfter,
	}, nil
}

// SignAddDelegatedSigner signs an addDelegatedSigner action.
// delegateAddress is a 0x-prefixed hex address. Permissions follows
// the API vocabulary ("trading", "delegate", "session").
// Pass expiresAt=0 for "never expires".
func (s *Signer) SignAddDelegatedSigner(
	subAccountID uint64,
	delegateAddress string,
	permissions []string,
	expiresAt int64,
	nonce uint64,
	expiresAfter int64,
) (*types.AddDelegatedSignerRequest, error) {
	if nonce == 0 {
		nonce = s.NextNonce()
	}
	expiresAfter = resolveExpiry(nonce, expiresAfter)

	typedData := eip712.BuildAddDelegatedSigner(subAccountID, delegateAddress, permissions, expiresAt, nonce, expiresAfter)
	sig, err := s.signTypedData(typedData)
	if err != nil {
		return nil, err
	}

	if permissions == nil {
		permissions = []string{}
	}

	return &types.AddDelegatedSignerRequest{
		Params: types.AddDelegatedSignerAction{
			Action:        "addDelegatedSigner",
			WalletAddress: delegateAddress,
			Permissions:   permissions,
			ExpiresAt:     expiresAt,
		},
		SubAccountID: uintStr(subAccountID),
		Nonce:        nonce,
		Signature:    sig,
		ExpiresAfter: expiresAfter,
	}, nil
}

// SignRemoveDelegatedSigner signs a removeDelegatedSigner action.
func (s *Signer) SignRemoveDelegatedSigner(
	subAccountID uint64,
	delegateAddress string,
	nonce uint64,
	expiresAfter int64,
) (*types.RemoveDelegatedSignerRequest, error) {
	if nonce == 0 {
		nonce = s.NextNonce()
	}
	expiresAfter = resolveExpiry(nonce, expiresAfter)

	typedData := eip712.BuildRemoveDelegatedSigner(subAccountID, delegateAddress, nonce, expiresAfter)
	sig, err := s.signTypedData(typedData)
	if err != nil {
		return nil, err
	}

	return &types.RemoveDelegatedSignerRequest{
		Params: map[string]any{
			"action":        "removeDelegatedSigner",
			"walletAddress": delegateAddress,
		},
		Signature:     sig,
		SubAccountID:  uintStr(subAccountID),
		WalletAddress: s.WalletAddress(),
		Nonce:         nonce,
		ExpiresAfter:  expiresAfter,
	}, nil
}

// SignRemoveAllDelegatedSigners signs a removeAllDelegatedSigners
// action.
func (s *Signer) SignRemoveAllDelegatedSigners(
	subAccountID uint64,
	nonce uint64,
	expiresAfter int64,
) (*types.RemoveAllDelegatedSignersRequest, error) {
	if nonce == 0 {
		nonce = s.NextNonce()
	}
	expiresAfter = resolveExpiry(nonce, expiresAfter)

	typedData := eip712.BuildRemoveAllDelegatedSigners(subAccountID, nonce, expiresAfter)
	sig, err := s.signTypedData(typedData)
	if err != nil {
		return nil, err
	}

	return &types.RemoveAllDelegatedSignersRequest{
		Params: types.RemoveAllDelegatedSignersAction{
			Action: "removeAllDelegatedSigners",
		},
		SubAccountID: uintStr(subAccountID),
		Nonce:        nonce,
		Signature:    sig,
		ExpiresAfter: expiresAfter,
	}, nil
}

// SignScheduleCancel signs a scheduleCancel (dead-man switch)
// action. Pass timeoutSeconds=0 to clear the schedule.
func (s *Signer) SignScheduleCancel(
	subAccountID uint64,
	timeoutSeconds int64,
	nonce uint64,
	expiresAfter int64,
) (*types.ScheduleCancelRequest, error) {
	if nonce == 0 {
		nonce = s.NextNonce()
	}
	expiresAfter = resolveExpiry(nonce, expiresAfter)

	typedData := eip712.BuildScheduleCancel(subAccountID, timeoutSeconds, nonce, expiresAfter)
	sig, err := s.signTypedData(typedData)
	if err != nil {
		return nil, err
	}

	return &types.ScheduleCancelRequest{
		Params: map[string]any{
			"action":         "scheduleCancel",
			"timeoutSeconds": timeoutSeconds,
		},
		Signature:     sig,
		SubAccountID:  uintStr(subAccountID),
		WalletAddress: s.WalletAddress(),
		Nonce:         nonce,
		ExpiresAfter:  expiresAfter,
	}, nil
}

// SignSubAccountAction signs a generic GET-style read action
// (getPositions, getOpenOrders, getSubAccount, getSubAccounts,
// getOrderHistory, getTrades, getFundingPayments,
// getPerformanceHistory). Pass nonce=0 to have the Signer pick a safe
// default.
func (s *Signer) SignSubAccountAction(
	subAccountID uint64,
	action string,
	nonce uint64,
	expiresAfter int64,
) (*types.SubAccountActionRequest, error) {
	if nonce == 0 {
		nonce = s.NextNonce()
	}
	expiresAfter = resolveExpiry(nonce, expiresAfter)

	typedData := eip712.BuildSubAccountAction(subAccountID, action, nonce, expiresAfter)
	sig, err := s.signTypedData(typedData)
	if err != nil {
		return nil, err
	}

	return &types.SubAccountActionRequest{
		Params: map[string]any{
			"action": action,
		},
		Signature:     sig,
		SubAccountID:  uintStr(subAccountID),
		WalletAddress: s.WalletAddress(),
		Nonce:         nonce,
		ExpiresAfter:  expiresAfter,
	}, nil
}

// SignAuthMessage signs the WebSocket auth-handshake AuthMessage.
// Returns the SignatureComponents directly (the WS auth wire format
// does not carry a SubAccountID / WalletAddress envelope; the
// caller assembles those alongside the signature). timestamp is
// milliseconds-since-epoch — pass 0 to use Now().
func (s *Signer) SignAuthMessage(
	subAccountID uint64,
	timestamp int64,
) (types.SignatureComponents, int64, error) {
	if timestamp == 0 {
		timestamp = time.Now().UnixMilli()
	}
	typedData := eip712.BuildAuthMessage(subAccountID, timestamp, eip712.ActionWebSocketAuth)
	sig, err := s.signTypedData(typedData)
	if err != nil {
		return types.SignatureComponents{}, 0, err
	}
	return sig, timestamp, nil
}
