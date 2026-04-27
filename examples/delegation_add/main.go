package main

import (
	"context"
	"fmt"
	"github.com/Fenway-snx/synthetix-go-sdk/examples/internal/config"
	"log"
)

func main() {
	ctx := context.Background()
	c, err := config.NewClient()
	if err != nil {
		log.Fatal(err)
	}
	defer c.Close()

	out, err := c.AddDelegatedSigner(ctx, "0x0000000000000000000000000000000000000000", []string{"trading"}, 0)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("%+v\n", out)
}
