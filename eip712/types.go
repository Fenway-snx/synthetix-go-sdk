package eip712

import "github.com/ethereum/go-ethereum/signer/core/apitypes"

// All type schemas below mirror the public signing contract
// byte-for-byte. The PrimaryType names listed in the comment are what
// each builder will set as TypedData.PrimaryType. Field order matters:
// HashStruct serialises the struct hash with the encoded fields in
// declaration order.

// AuthMessageTypes is the schema for the WebSocket auth handshake.
// Primary type: AuthMessage.
func AuthMessageTypes() apitypes.Types {
	return apitypes.Types{
		"EIP712Domain": DomainFields(),
		"AuthMessage": {
			{Name: "subAccountId", Type: "uint256"},
			{Name: "timestamp", Type: "uint256"},
			{Name: "action", Type: "string"},
		},
	}
}

// PlaceOrdersTypes is the schema for placeOrders. Primary type:
// PlaceOrders. Note the nested Order array struct.
func PlaceOrdersTypes() apitypes.Types {
	return apitypes.Types{
		"EIP712Domain": DomainFields(),
		"Order": {
			{Name: "symbol", Type: "string"},
			{Name: "side", Type: "string"},
			{Name: "orderType", Type: "string"},
			{Name: "price", Type: "string"},
			{Name: "triggerPrice", Type: "string"},
			{Name: "quantity", Type: "string"},
			{Name: "reduceOnly", Type: "bool"},
			{Name: "isTriggerMarket", Type: "bool"},
			{Name: "clientOrderId", Type: "string"},
			{Name: "closePosition", Type: "bool"},
		},
		"PlaceOrders": {
			{Name: "subAccountId", Type: "uint256"},
			{Name: "orders", Type: "Order[]"},
			{Name: "grouping", Type: "string"},
			{Name: "nonce", Type: "uint256"},
			{Name: "expiresAfter", Type: "uint256"},
		},
	}
}

// CancelOrdersTypes is the schema for cancelOrders by venue order id.
func CancelOrdersTypes() apitypes.Types {
	return apitypes.Types{
		"EIP712Domain": DomainFields(),
		"CancelOrders": {
			{Name: "subAccountId", Type: "uint256"},
			{Name: "orderIds", Type: "uint256[]"},
			{Name: "nonce", Type: "uint256"},
			{Name: "expiresAfter", Type: "uint256"},
		},
	}
}

// CancelOrdersByCloidTypes is the schema for cancelOrders by client
// order id.
func CancelOrdersByCloidTypes() apitypes.Types {
	return apitypes.Types{
		"EIP712Domain": DomainFields(),
		"CancelOrdersByCloid": {
			{Name: "subAccountId", Type: "uint256"},
			{Name: "clientOrderIds", Type: "string[]"},
			{Name: "nonce", Type: "uint256"},
			{Name: "expiresAfter", Type: "uint256"},
		},
	}
}

// CancelAllOrdersTypes is the schema for cancelAllOrders.
func CancelAllOrdersTypes() apitypes.Types {
	return apitypes.Types{
		"EIP712Domain": DomainFields(),
		"CancelAllOrders": {
			{Name: "subAccountId", Type: "uint256"},
			{Name: "symbols", Type: "string[]"},
			{Name: "nonce", Type: "uint256"},
			{Name: "expiresAfter", Type: "uint256"},
		},
	}
}

// ModifyOrderTypes is the schema for modifyOrder by venue order id.
func ModifyOrderTypes() apitypes.Types {
	return apitypes.Types{
		"EIP712Domain": DomainFields(),
		"ModifyOrder": {
			{Name: "subAccountId", Type: "uint256"},
			{Name: "orderId", Type: "uint256"},
			{Name: "price", Type: "string"},
			{Name: "quantity", Type: "string"},
			{Name: "triggerPrice", Type: "string"},
			{Name: "nonce", Type: "uint256"},
			{Name: "expiresAfter", Type: "uint256"},
		},
	}
}

// ModifyOrderByCloidTypes is the schema for modifyOrder by client
// order id.
func ModifyOrderByCloidTypes() apitypes.Types {
	return apitypes.Types{
		"EIP712Domain": DomainFields(),
		"ModifyOrderByCloid": {
			{Name: "subAccountId", Type: "uint256"},
			{Name: "clientOrderId", Type: "string"},
			{Name: "price", Type: "string"},
			{Name: "quantity", Type: "string"},
			{Name: "triggerPrice", Type: "string"},
			{Name: "nonce", Type: "uint256"},
			{Name: "expiresAfter", Type: "uint256"},
		},
	}
}

// UpdateLeverageTypes is the schema for updateLeverage.
func UpdateLeverageTypes() apitypes.Types {
	return apitypes.Types{
		"EIP712Domain": DomainFields(),
		"UpdateLeverage": {
			{Name: "subAccountId", Type: "uint256"},
			{Name: "symbol", Type: "string"},
			{Name: "leverage", Type: "string"},
			{Name: "nonce", Type: "uint256"},
			{Name: "expiresAfter", Type: "uint256"},
		},
	}
}

// WithdrawCollateralTypes is the schema for withdrawCollateral.
func WithdrawCollateralTypes() apitypes.Types {
	return apitypes.Types{
		"EIP712Domain": DomainFields(),
		"WithdrawCollateral": {
			{Name: "subAccountId", Type: "uint256"},
			{Name: "symbol", Type: "string"},
			{Name: "amount", Type: "string"},
			{Name: "destination", Type: "address"},
			{Name: "nonce", Type: "uint256"},
			{Name: "expiresAfter", Type: "uint256"},
		},
	}
}

// CreateSubaccountTypes is the schema for createSubaccount. Note
// masterSubAccountId rather than subAccountId — the field carries
// the existing subaccount the caller proves ownership of.
func CreateSubaccountTypes() apitypes.Types {
	return apitypes.Types{
		"EIP712Domain": DomainFields(),
		"CreateSubaccount": {
			{Name: "masterSubAccountId", Type: "uint256"},
			{Name: "name", Type: "string"},
			{Name: "nonce", Type: "uint256"},
			{Name: "expiresAfter", Type: "uint256"},
		},
	}
}

// TransferCollateralTypes is the schema for transferCollateral.
// Field order is alphabetical to match the public signing contract,
// not the conventional subAccountId-first ordering.
func TransferCollateralTypes() apitypes.Types {
	return apitypes.Types{
		"EIP712Domain": DomainFields(),
		"TransferCollateral": {
			{Name: "amount", Type: "string"},
			{Name: "expiresAfter", Type: "uint256"},
			{Name: "nonce", Type: "uint256"},
			{Name: "subAccountId", Type: "uint256"},
			{Name: "symbol", Type: "string"},
			{Name: "to", Type: "uint256"},
		},
	}
}

// UpdateSubAccountNameTypes is the schema for updateSubAccountName.
func UpdateSubAccountNameTypes() apitypes.Types {
	return apitypes.Types{
		"EIP712Domain": DomainFields(),
		"UpdateSubAccountName": {
			{Name: "subAccountId", Type: "uint256"},
			{Name: "name", Type: "string"},
			{Name: "nonce", Type: "uint256"},
			{Name: "expiresAfter", Type: "uint256"},
		},
	}
}

// AddDelegatedSignerTypes is the schema for addDelegatedSigner.
func AddDelegatedSignerTypes() apitypes.Types {
	return apitypes.Types{
		"EIP712Domain": DomainFields(),
		"AddDelegatedSigner": {
			{Name: "delegateAddress", Type: "address"},
			{Name: "subAccountId", Type: "uint256"},
			{Name: "nonce", Type: "uint256"},
			{Name: "expiresAfter", Type: "uint256"},
			{Name: "expiresAt", Type: "uint256"},
			{Name: "permissions", Type: "string[]"},
		},
	}
}

// RemoveDelegatedSignerTypes is the schema for removeDelegatedSigner.
func RemoveDelegatedSignerTypes() apitypes.Types {
	return apitypes.Types{
		"EIP712Domain": DomainFields(),
		"RemoveDelegatedSigner": {
			{Name: "delegateAddress", Type: "address"},
			{Name: "subAccountId", Type: "uint256"},
			{Name: "nonce", Type: "uint256"},
			{Name: "expiresAfter", Type: "uint256"},
		},
	}
}

// RemoveAllDelegatedSignersTypes is the schema for
// removeAllDelegatedSigners.
func RemoveAllDelegatedSignersTypes() apitypes.Types {
	return apitypes.Types{
		"EIP712Domain": DomainFields(),
		"RemoveAllDelegatedSigners": {
			{Name: "subAccountId", Type: "uint256"},
			{Name: "nonce", Type: "uint256"},
			{Name: "expiresAfter", Type: "uint256"},
		},
	}
}

// ScheduleCancelTypes is the schema for scheduleCancel (dead-man
// switch).
func ScheduleCancelTypes() apitypes.Types {
	return apitypes.Types{
		"EIP712Domain": DomainFields(),
		"ScheduleCancel": {
			{Name: "subAccountId", Type: "uint256"},
			{Name: "timeoutSeconds", Type: "uint256"},
			{Name: "nonce", Type: "uint256"},
			{Name: "expiresAfter", Type: "uint256"},
		},
	}
}

// SubAccountActionTypes is the schema for the generic
// SubAccountAction envelope used by authenticated GET-style reads
// (getPositions, getOpenOrders, etc).
//
// includeNonce controls whether the schema declares the optional
// nonce field. The API accepts both shapes for backwards
// compatibility with older clients; new callers should pass
// includeNonce=true.
func SubAccountActionTypes(includeNonce bool) apitypes.Types {
	fields := []apitypes.Type{
		{Name: "subAccountId", Type: "uint256"},
		{Name: "action", Type: "string"},
	}
	if includeNonce {
		fields = append(fields, apitypes.Type{Name: "nonce", Type: "uint256"})
	}
	fields = append(fields, apitypes.Type{Name: "expiresAfter", Type: "uint256"})

	return apitypes.Types{
		"EIP712Domain": DomainFields(),
		"SubAccountAction": fields,
	}
}
