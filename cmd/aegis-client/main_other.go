//go:build !windows

package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Println("Aegis client runs only on Windows.")
	os.Exit(1)
}
