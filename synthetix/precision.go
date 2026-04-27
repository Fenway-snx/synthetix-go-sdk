package synthetix

import (
	"context"
	"errors"
	"fmt"

	"github.com/shopspring/decimal"
)

// FormatPrice truncates price to the market's price increment.
func (c *Client) FormatPrice(ctx context.Context, symbol, price string) (string, error) {
	market, err := c.info.GetMarket(ctx, symbol)
	if err != nil {
		return "", err
	}
	return formatToIncrement(price, market.PriceIncrement)
}

// FormatSize truncates quantity to the market's order-size increment.
func (c *Client) FormatSize(ctx context.Context, symbol, quantity string) (string, error) {
	market, err := c.info.GetMarket(ctx, symbol)
	if err != nil {
		return "", err
	}
	return formatToIncrement(quantity, market.OrderSizeIncrement)
}

// FormatQuantity is an alias for FormatSize.
func (c *Client) FormatQuantity(ctx context.Context, symbol, quantity string) (string, error) {
	return c.FormatSize(ctx, symbol, quantity)
}

func formatToIncrement(value, increment string) (string, error) {
	v, err := decimal.NewFromString(value)
	if err != nil {
		return "", fmt.Errorf("synthetix: parse value %q: %w", value, err)
	}
	inc, err := decimal.NewFromString(increment)
	if err != nil {
		return "", fmt.Errorf("synthetix: parse increment %q: %w", increment, err)
	}
	if !inc.IsPositive() {
		return "", errors.New("synthetix: increment must be positive")
	}
	places := int32(0)
	if inc.Exponent() < 0 {
		places = -inc.Exponent()
	}
	return v.Div(inc).Floor().Mul(inc).StringFixed(places), nil
}
