// Package synthetix is the convenience entry point for the Synthetix
// V4 Go SDK.
//
// It bundles the lower-level clients (sdk/restinfo, sdk/resttrade,
// sdk/wsinfo) into a single Client constructed from one Config. Use
// it when you want a one-liner for the common case; reach for the
// individual subpackages directly when you need finer control.
//
//	c, err := synthetix.NewClient(synthetix.Config{
//	    BaseURL: "https://api.synthetix.io",
//	})
//	if err != nil { ... }
//	defer c.Close()
//
//	markets, err := c.Info().GetMarkets(ctx)
//
// The SDK is BYO-logger. Pass any sdk/logger.Logger implementation;
// nil is acceptable and silently drops output.
package synthetix
