package main

import (
	"context"
	"fmt"
	"github.com/Fenway-snx/synthetix-go-sdk/examples/internal/config"
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

	out, err := c.TwapOrder(ctx, "BTC-USDT", synthetix.SideBuy, "0.01", 3600, "50000")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("%+v\n", out)
}
