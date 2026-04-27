package main

import (
	"context"
	"fmt"
	"github.com/Fenway-snx/synthetix-go-sdk/examples/internal/config"
	"github.com/Fenway-snx/synthetix-go-sdk/signer"
	"github.com/Fenway-snx/synthetix-go-sdk/synthetix"
	"log"
)

func main() {
	ctx := context.Background()
	c, err := config.NewClient()
	if err != nil {
		log.Fatal(err)
	}
	defer c.Close()

	out, err := c.PlaceOrders(ctx, []signer.PlaceOrderInput{{Symbol: "BTC-USDT", Side: synthetix.SideBuy, OrderType: synthetix.OrderTypeLimitGTC, Price: "50000", Quantity: "0.01"}})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("%+v\n", out)
}
