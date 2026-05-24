package types

import (
	"encoding/json"
	"testing"
)

func TestMaintenanceMarginTierAcceptsNumericMaxLeverage(t *testing.T) {
	var tier MaintenanceMarginTier
	err := json.Unmarshal([]byte(`{
		"minPositionSize": "0",
		"maxPositionSize": "15000000",
		"maxLeverage": 50,
		"initialMarginRequirement": "0.02",
		"maintenanceMarginRequirement": "0.01",
		"maintenanceDeductionValue": "0"
	}`), &tier)
	if err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if tier.MaxLeverage != "50" {
		t.Fatalf("MaxLeverage = %q, want 50", tier.MaxLeverage)
	}
	if tier.MinPositionSize != "0" || tier.MaintenanceMarginRequirement != "0.01" {
		t.Fatalf("tier fields not populated: %+v", tier)
	}
}

func TestWSMessageMetAcceptsString(t *testing.T) {
	var msg WSMessage
	err := json.Unmarshal([]byte(`{"channel":"subAccountUpdate","met":"1716566400123"}`), &msg)
	if err != nil {
		t.Fatalf("unmarshal string met: %v", err)
	}
	if msg.Met != 1716566400123 {
		t.Fatalf("Met = %d, want 1716566400123", msg.Met)
	}

	out, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if string(out) != `{"channel":"subAccountUpdate","met":"1716566400123"}` {
		t.Fatalf("marshal = %s", out)
	}
}
