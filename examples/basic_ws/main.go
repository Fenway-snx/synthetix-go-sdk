package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/Fenway-snx/synthetix-go-sdk/examples/internal/config"
	"github.com/Fenway-snx/synthetix-go-sdk/synthetix"
	"github.com/Fenway-snx/synthetix-go-sdk/types"
)

func main() {
	ctx := context.Background()
	c, err := config.NewClient()
	if err != nil {
		log.Fatal(err)
	}
	defer c.Close()

	stopOrderbook, err := c.SubscribeOrderbook(ctx, "BTC-USDT", printMessage, synthetix.WithDepth(10))
	if err != nil {
		log.Fatal(err)
	}
	defer stopOrderbook()

	stopTrades, err := c.SubscribeTrades(ctx, "BTC-USDT", printMessage)
	if err != nil {
		log.Fatal(err)
	}
	defer stopTrades()

	time.Sleep(30 * time.Second)
}

func printMessage(msg *types.WSMessage) {
	fmt.Println(string(msg.Raw))
}
