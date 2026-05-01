package main

import (
	"context"
	"fmt"
	"github.com/synthetixio/synthetix-go/examples/internal/config"
	"github.com/synthetixio/synthetix-go/types"
	"log"
)

func main() {
	ctx := context.Background()
	c, err := config.NewClient()
	if err != nil {
		log.Fatal(err)
	}
	defer c.Close()

	out, err := c.WSTrade().SubscribeSubAccountUpdates(ctx, 1, func(*types.WSMessage) {})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("subscribed: %T\n", out)
}
