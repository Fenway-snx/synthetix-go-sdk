package main

import (
	"context"
	"fmt"
	"github.com/synthetixio/synthetix-go/examples/internal/config"
	"log"
)

func main() {
	ctx := context.Background()
	c, err := config.NewClient()
	if err != nil {
		log.Fatal(err)
	}
	defer c.Close()

	out, err := c.ModifyOrder(ctx, 123, "50001", "0.01", "")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("%+v\n", out)
}
