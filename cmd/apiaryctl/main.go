package main

import (
	"fmt"
	"os"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintf(os.Stderr, "Usage: apiaryctl <command> [options]\n")
		os.Exit(1)
	}

	// TODO: Implement CLI commands
	fmt.Println("apiaryctl - CLI implementation pending")
}
