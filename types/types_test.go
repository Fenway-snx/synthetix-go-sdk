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
