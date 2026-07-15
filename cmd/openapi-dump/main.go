// Command openapi-dump emits the daemon's OpenAPI 3.1 spec to stdout.
// It builds the router with both admin flags forced on (so the spec covers
// every operation) and prints what huma rendered from the live registrations.
package main

import (
	"fmt"
	"os"

	"sim7600d/internal/api"
)

func main() {
	srv := api.NewServer(api.Config{
		AuthToken:          "openapi-dump-noop",
		AllowATPassthrough: true,
		AllowModemReset:    true,
		// Modem and Store are nil; operations don't execute during dump,
		// only their registration metadata is serialized.
	})
	if _, err := os.Stdout.Write(srv.Spec); err != nil {
		fmt.Fprintf(os.Stderr, "openapi-dump: %v\n", err)
		os.Exit(1)
	}
	if _, err := os.Stdout.WriteString("\n"); err != nil {
		fmt.Fprintf(os.Stderr, "openapi-dump: %v\n", err)
		os.Exit(1)
	}
}
