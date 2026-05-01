package main

import (
	"context"
	"fmt"
	"github.com/synthetixio/synthetix-go/examples/internal/config"
	"github.com/synthetixio/synthetix-go/synthetix"
	"log"
)

func main() {
	ctx := context.Background()
	c, err := config.NewClient()
	if err != nil {
		log.Fatal(err)
	}
	defer c.Close()

	out, err := c.CancelOrder(ctx, 123, synthetix.OverWS())
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("%+v\n", out)
}
