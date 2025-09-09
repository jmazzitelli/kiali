package main

import (
	"os"

	"github.com/kiali/kiali-integration-framework/internal/cli"
)

func main() {
	rootCmd := cli.NewRootCommand()

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
