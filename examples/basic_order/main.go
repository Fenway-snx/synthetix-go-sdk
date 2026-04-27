package main

import (
	"context"
	"fmt"
	"log"
	"strconv"

	"github.com/Fenway-snx/synthetix-go-sdk/examples/internal/config"
	"github.com/Fenway-snx/synthetix-go-sdk/synthetix"
)

func main() {
	ctx := context.Background()
	c, err := config.NewClient()
	if err != nil {
		log.Fatal(err)
	}
	defer c.Close()

	positions, err := c.GetPositions(ctx)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("open positions: %d\n", len(positions))
	for _, position := range positions {
		fmt.Printf("%s %s %s\n", position.Symbol, position.Side, position.Quantity)
	}

	price, err := c.FormatPrice(ctx, "BTC-USDT", "1000")
	if err != nil {
		log.Fatal(err)
	}
	size, err := c.FormatSize(ctx, "BTC-USDT", "0.01")
	if err != nil {
		log.Fatal(err)
	}

	order, err := c.PlaceOrder(ctx, "BTC-USDT", synthetix.SideBuy, synthetix.OrderTypeLimit, price, size, synthetix.GTC())
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("place order: %+v\n", order)

	if len(order.Statuses) == 0 || order.Statuses[0].Resting == nil {
		return
	}
	venueID := order.Statuses[0].Resting.OrderID.VenueID
	id, err := strconv.ParseUint(venueID, 10, 64)
	if err != nil {
		log.Fatal(err)
	}
	cancel, err := c.CancelOrder(ctx, id)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("cancel order: %+v\n", cancel)
}
