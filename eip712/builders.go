package eip712

import (
	"strconv"

	"github.com/ethereum/go-ethereum/common/math"
	"github.com/ethereum/go-ethereum/signer/core/apitypes"
)

// Action constants used as the `action` field on signed envelopes and
// inside specific typed-data messages.
const (
	// ActionWebSocketAuth is the action string carried inside the
	// AuthMessage typed-data sent to authenticate a WebSocket
	// session.
	ActionWebSocketAuth = "websocket_auth"
)

// PlaceOrderItem is the unsigned input shape for one order inside a
// PlaceOrders message. Wire shape is the EIP-712 Order struct from
// PlaceOrdersTypes; field order in the JSON output does not affect
// the digest because HashStruct iterates declaration order from the
// schema. ClientOrderID may be empty.
type PlaceOrderItem struct {
	Symbol          string
	Side            string
	OrderType       string
	Price           string
	TriggerPrice    string
	Quantity        string
	ReduceOnly      bool
	IsTriggerMarket bool
	ClientOrderID   string
	ClosePosition   bool
}

// BuildPlaceOrders returns the typed-data for a placeOrders action.
//
// Parameters mirror the canonical /v1/trade payload one-for-one;
// grouping is "na" for normal orders, "positionTpsl" / "twap" for
// the corresponding compound orders.
func BuildPlaceOrders(
	subAccountID uint64,
	orders []PlaceOrderItem,
	grouping string,
	nonce uint64,
	expiresAfter int64,
) apitypes.TypedData {
	encoded := make([]map[string]any, len(orders))
	for i, o := range orders {
		encoded[i] = map[string]any{
			"symbol":          o.Symbol,
			"side":            o.Side,
			"orderType":       o.OrderType,
			"price":           o.Price,
			"triggerPrice":    o.TriggerPrice,
			"quantity":        o.Quantity,
			"reduceOnly":      o.ReduceOnly,
			"isTriggerMarket": o.IsTriggerMarket,
			"clientOrderId":   o.ClientOrderID,
			"closePosition":   o.ClosePosition,
		}
	}
	return apitypes.TypedData{
		Types:       PlaceOrdersTypes(),
		PrimaryType: "PlaceOrders",
		Domain:      DefaultDomain(),
		Message: apitypes.TypedDataMessage{
			"subAccountId": uintStr(subAccountID),
			"orders":       encoded,
			"grouping":     grouping,
			"nonce":        uintStr(nonce),
			"expiresAfter": int64Str(expiresAfter),
		},
	}
}

// BuildCancelOrders returns the typed-data for cancelOrders by
// venue order id.
func BuildCancelOrders(
	subAccountID uint64,
	orderIDs []uint64,
	nonce uint64,
	expiresAfter int64,
) apitypes.TypedData {
	encoded := make([]any, len(orderIDs))
	for i, id := range orderIDs {
		encoded[i] = math.NewHexOrDecimal256(int64(id))
	}
	return apitypes.TypedData{
		Types:       CancelOrdersTypes(),
		PrimaryType: "CancelOrders",
		Domain:      DefaultDomain(),
		Message: apitypes.TypedDataMessage{
			"subAccountId": uintStr(subAccountID),
			"orderIds":     encoded,
			"nonce":        uintStr(nonce),
			"expiresAfter": int64Str(expiresAfter),
		},
	}
}

// BuildCancelOrdersByCloid returns the typed-data for cancelOrders
// by client order id.
func BuildCancelOrdersByCloid(
	subAccountID uint64,
	clientOrderIDs []string,
	nonce uint64,
	expiresAfter int64,
) apitypes.TypedData {
	return apitypes.TypedData{
		Types:       CancelOrdersByCloidTypes(),
		PrimaryType: "CancelOrdersByCloid",
		Domain:      DefaultDomain(),
		Message: apitypes.TypedDataMessage{
			"subAccountId":   uintStr(subAccountID),
			"clientOrderIds": clientOrderIDs,
			"nonce":          uintStr(nonce),
			"expiresAfter":   int64Str(expiresAfter),
		},
	}
}

// BuildCancelAllOrders returns the typed-data for cancelAllOrders.
// symbols may be nil/empty for "cancel everything"; non-empty
// restricts the cancel to the listed markets.
func BuildCancelAllOrders(
	subAccountID uint64,
	symbols []string,
	nonce uint64,
	expiresAfter int64,
) apitypes.TypedData {
	if symbols == nil {
		symbols = []string{}
	}
	return apitypes.TypedData{
		Types:       CancelAllOrdersTypes(),
		PrimaryType: "CancelAllOrders",
		Domain:      DefaultDomain(),
		Message: apitypes.TypedDataMessage{
			"subAccountId": uintStr(subAccountID),
			"symbols":      symbols,
			"nonce":        uintStr(nonce),
			"expiresAfter": int64Str(expiresAfter),
		},
	}
}

// BuildModifyOrder returns the typed-data for modifyOrder by venue
// order id. Pass empty string for any field that is not being
// modified — the API treats empty as "leave unchanged".
func BuildModifyOrder(
	subAccountID, orderID uint64,
	price, quantity, triggerPrice string,
	nonce uint64,
	expiresAfter int64,
) apitypes.TypedData {
	return apitypes.TypedData{
		Types:       ModifyOrderTypes(),
		PrimaryType: "ModifyOrder",
		Domain:      DefaultDomain(),
		Message: apitypes.TypedDataMessage{
			"subAccountId": uintStr(subAccountID),
			"orderId":      uintStr(orderID),
			"price":        price,
			"quantity":     quantity,
			"triggerPrice": triggerPrice,
			"nonce":        uintStr(nonce),
			"expiresAfter": int64Str(expiresAfter),
		},
	}
}

// BuildModifyOrderByCloid returns the typed-data for modifyOrder by
// client order id.
func BuildModifyOrderByCloid(
	subAccountID uint64,
	clientOrderID, price, quantity, triggerPrice string,
	nonce uint64,
	expiresAfter int64,
) apitypes.TypedData {
	return apitypes.TypedData{
		Types:       ModifyOrderByCloidTypes(),
		PrimaryType: "ModifyOrderByCloid",
		Domain:      DefaultDomain(),
		Message: apitypes.TypedDataMessage{
			"subAccountId":  uintStr(subAccountID),
			"clientOrderId": clientOrderID,
			"price":         price,
			"quantity":      quantity,
			"triggerPrice":  triggerPrice,
			"nonce":         uintStr(nonce),
			"expiresAfter":  int64Str(expiresAfter),
		},
	}
}

// BuildUpdateLeverage returns the typed-data for updateLeverage.
// Leverage is a decimal-string multiplier (e.g. "5", "10.5").
func BuildUpdateLeverage(
	subAccountID uint64,
	symbol, leverage string,
	nonce uint64,
	expiresAfter int64,
) apitypes.TypedData {
	return apitypes.TypedData{
		Types:       UpdateLeverageTypes(),
		PrimaryType: "UpdateLeverage",
		Domain:      DefaultDomain(),
		Message: apitypes.TypedDataMessage{
			"subAccountId": uintStr(subAccountID),
			"symbol":       symbol,
			"leverage":     leverage,
			"nonce":        uintStr(nonce),
			"expiresAfter": int64Str(expiresAfter),
		},
	}
}

// BuildWithdrawCollateral returns the typed-data for
// withdrawCollateral. destination is a 0x-prefixed hex address.
func BuildWithdrawCollateral(
	subAccountID uint64,
	symbol, amount, destination string,
	nonce uint64,
	expiresAfter int64,
) apitypes.TypedData {
	return apitypes.TypedData{
		Types:       WithdrawCollateralTypes(),
		PrimaryType: "WithdrawCollateral",
		Domain:      DefaultDomain(),
		Message: apitypes.TypedDataMessage{
			"subAccountId": uintStr(subAccountID),
			"symbol":       symbol,
			"amount":       amount,
			"destination":  destination,
			"nonce":        uintStr(nonce),
			"expiresAfter": int64Str(expiresAfter),
		},
	}
}

// BuildCreateSubaccount returns the typed-data for createSubaccount.
// masterSubAccountID is the existing subaccount whose ownership the
// caller is proving — the new subaccount's id is assigned by the API.
func BuildCreateSubaccount(
	masterSubAccountID uint64,
	name string,
	nonce uint64,
	expiresAfter int64,
) apitypes.TypedData {
	return apitypes.TypedData{
		Types:       CreateSubaccountTypes(),
		PrimaryType: "CreateSubaccount",
		Domain:      DefaultDomain(),
		Message: apitypes.TypedDataMessage{
			"masterSubAccountId": uintStr(masterSubAccountID),
			"name":               name,
			"nonce":              uintStr(nonce),
			"expiresAfter":       int64Str(expiresAfter),
		},
	}
}

// BuildTransferCollateral returns the typed-data for
// transferCollateral. amount is a decimal string; toSubAccountID is
// the destination subaccount on the same wallet.
func BuildTransferCollateral(
	subAccountID, toSubAccountID uint64,
	symbol, amount string,
	nonce uint64,
	expiresAfter int64,
) apitypes.TypedData {
	return apitypes.TypedData{
		Types:       TransferCollateralTypes(),
		PrimaryType: "TransferCollateral",
		Domain:      DefaultDomain(),
		Message: apitypes.TypedDataMessage{
			"amount":       amount,
			"expiresAfter": int64Str(expiresAfter),
			"nonce":        uintStr(nonce),
			"subAccountId": uintStr(subAccountID),
			"symbol":       symbol,
			"to":           uintStr(toSubAccountID),
		},
	}
}

// BuildUpdateSubAccountName returns the typed-data for
// updateSubAccountName.
func BuildUpdateSubAccountName(
	subAccountID uint64,
	name string,
	nonce uint64,
	expiresAfter int64,
) apitypes.TypedData {
	return apitypes.TypedData{
		Types:       UpdateSubAccountNameTypes(),
		PrimaryType: "UpdateSubAccountName",
		Domain:      DefaultDomain(),
		Message: apitypes.TypedDataMessage{
			"subAccountId": uintStr(subAccountID),
			"name":         name,
			"nonce":        uintStr(nonce),
			"expiresAfter": int64Str(expiresAfter),
		},
	}
}

// BuildAddDelegatedSigner returns the typed-data for
// addDelegatedSigner. delegateAddress is a 0x-prefixed hex address.
// permissions may be nil; expiresAt may be 0 for "never expires".
func BuildAddDelegatedSigner(
	subAccountID uint64,
	delegateAddress string,
	permissions []string,
	expiresAt int64,
	nonce uint64,
	expiresAfter int64,
) apitypes.TypedData {
	if permissions == nil {
		permissions = []string{}
	}
	return apitypes.TypedData{
		Types:       AddDelegatedSignerTypes(),
		PrimaryType: "AddDelegatedSigner",
		Domain:      DefaultDomain(),
		Message: apitypes.TypedDataMessage{
			"delegateAddress": delegateAddress,
			"subAccountId":    uintStr(subAccountID),
			"nonce":           uintStr(nonce),
			"expiresAfter":    int64Str(expiresAfter),
			"expiresAt":       int64Str(expiresAt),
			"permissions":     permissions,
		},
	}
}

// BuildRemoveDelegatedSigner returns the typed-data for
// removeDelegatedSigner.
func BuildRemoveDelegatedSigner(
	subAccountID uint64,
	delegateAddress string,
	nonce uint64,
	expiresAfter int64,
) apitypes.TypedData {
	return apitypes.TypedData{
		Types:       RemoveDelegatedSignerTypes(),
		PrimaryType: "RemoveDelegatedSigner",
		Domain:      DefaultDomain(),
		Message: apitypes.TypedDataMessage{
			"delegateAddress": delegateAddress,
			"subAccountId":    uintStr(subAccountID),
			"nonce":           uintStr(nonce),
			"expiresAfter":    int64Str(expiresAfter),
		},
	}
}

// BuildRemoveAllDelegatedSigners returns the typed-data for
// removeAllDelegatedSigners.
func BuildRemoveAllDelegatedSigners(
	subAccountID uint64,
	nonce uint64,
	expiresAfter int64,
) apitypes.TypedData {
	return apitypes.TypedData{
		Types:       RemoveAllDelegatedSignersTypes(),
		PrimaryType: "RemoveAllDelegatedSigners",
		Domain:      DefaultDomain(),
		Message: apitypes.TypedDataMessage{
			"subAccountId": uintStr(subAccountID),
			"nonce":        uintStr(nonce),
			"expiresAfter": int64Str(expiresAfter),
		},
	}
}

// BuildScheduleCancel returns the typed-data for scheduleCancel
// (dead-man switch).
func BuildScheduleCancel(
	subAccountID uint64,
	timeoutSeconds int64,
	nonce uint64,
	expiresAfter int64,
) apitypes.TypedData {
	return apitypes.TypedData{
		Types:       ScheduleCancelTypes(),
		PrimaryType: "ScheduleCancel",
		Domain:      DefaultDomain(),
		Message: apitypes.TypedDataMessage{
			"subAccountId":   uintStr(subAccountID),
			"timeoutSeconds": int64Str(timeoutSeconds),
			"nonce":          uintStr(nonce),
			"expiresAfter":   int64Str(expiresAfter),
		},
	}
}

// BuildAuthMessage returns the typed-data for the WebSocket auth
// handshake. timestamp is seconds-since-epoch.
//
// Uses DefaultDomain() — production callers on api.synthetix.io
// should use this overload. Self-hosted deployments on a
// non-default chain or domain should call BuildAuthMessageWithDomain.
func BuildAuthMessage(
	subAccountID uint64,
	timestamp int64,
	action string,
) apitypes.TypedData {
	return BuildAuthMessageWithDomain(subAccountID, timestamp, action, DefaultDomain())
}

// BuildAuthMessageWithDomain is BuildAuthMessage with an explicit
// domain. Useful for self-hosted deployments that override the
// chain/version triple.
func BuildAuthMessageWithDomain(
	subAccountID uint64,
	timestamp int64,
	action string,
	domain apitypes.TypedDataDomain,
) apitypes.TypedData {
	return apitypes.TypedData{
		Types:       AuthMessageTypes(),
		PrimaryType: "AuthMessage",
		Domain:      domain,
		Message: apitypes.TypedDataMessage{
			"subAccountId": uintStr(subAccountID),
			"timestamp":    int64Str(timestamp),
			"action":       action,
		},
	}
}

// BuildSubAccountAction returns the typed-data for the generic
// authenticated GET-style action envelope (getPositions,
// getOpenOrders, etc). nonce of 0 omits the nonce field from the
// schema (older-client compatibility); pass a non-zero nonce to use
// the canonical schema.
func BuildSubAccountAction(
	subAccountID uint64,
	action string,
	nonce uint64,
	expiresAfter int64,
) apitypes.TypedData {
	includeNonce := nonce > 0
	message := apitypes.TypedDataMessage{
		"subAccountId": uintStr(subAccountID),
		"action":       action,
		"expiresAfter": int64Str(expiresAfter),
	}
	if includeNonce {
		message["nonce"] = uintStr(nonce)
	}
	return apitypes.TypedData{
		Types:       SubAccountActionTypes(includeNonce),
		PrimaryType: "SubAccountAction",
		Domain:      DefaultDomain(),
		Message:     message,
	}
}

// uintStr / int64Str produce decimal-string encodings that match the
// public signing contract byte-for-byte.
// EIP-712 HashStruct accepts strings for uint256 fields via its
// numeric coercion path, so the digest is identical whether you
// hand in "42", *math.HexOrDecimal256(42), or the literal int 42.

func uintStr(v uint64) string  { return strconv.FormatUint(v, 10) }
func int64Str(v int64) string  { return strconv.FormatInt(v, 10) }
