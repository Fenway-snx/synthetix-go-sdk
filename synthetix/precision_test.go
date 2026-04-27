package synthetix

import "testing"

func TestFormatToIncrement(t *testing.T) {
	tests := []struct {
		name      string
		value     string
		increment string
		want      string
	}{
		{name: "price", value: "50000.019", increment: "0.01", want: "50000.01"},
		{name: "size", value: "0.123456", increment: "0.001", want: "0.123"},
		{name: "whole", value: "12.9", increment: "1", want: "12"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := formatToIncrement(tt.value, tt.increment)
			if err != nil {
				t.Fatalf("formatToIncrement: %v", err)
			}
			if got != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestCloseSide(t *testing.T) {
	if got, err := closeSide(SideBuy); err != nil || got != SideSell {
		t.Fatalf("buy close side got %q err=%v", got, err)
	}
	if got, err := closeSide("short"); err != nil || got != SideBuy {
		t.Fatalf("short close side got %q err=%v", got, err)
	}
}
