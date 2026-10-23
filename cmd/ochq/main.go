// Command ochq exposes a local port through an ochqd server.
//
//	ochq 3000
//	ochq http 8080
//	ochq http 3000 --sub my-api --server tunnel.ochq.uz:9000
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/mukhiddinov/ochq/internal/client"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cmd := newRootCmd(client.Run, os.Stdout)
	cmd.SetArgs(compatArgs(os.Args[1:]))
	if err := cmd.ExecuteContext(ctx); err != nil {
		fmt.Fprintln(os.Stderr, "ochq:", err)
		if hint := hintFor(err); hint != "" {
			fmt.Fprintln(os.Stderr, "hint:", hint)
		}
		os.Exit(1)
	}
}
