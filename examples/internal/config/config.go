package config

import (
	"github.com/synthetixio/synthetix-go/synthetix"
)

// NewClient builds a client from environment variables shared by all
// examples. Network calls are intentionally left to each example's
// main package so `go test ./...` can compile examples offline.
func NewClient() (*synthetix.Client, error) {
	return synthetix.NewClientFromEnv()
}

