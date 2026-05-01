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

	out, err := c.RemoveDelegatedSigner(ctx, "0x0000000000000000000000000000000000000000")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("%+v\n", out)
}
