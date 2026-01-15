package main

import (
	"os"

	"github.com/agentapiary/apiary/cmd/apiaryctl/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}
