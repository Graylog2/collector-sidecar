package main

import (
	"fmt"
	"os"
	"time"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("No command line options given")
		os.Exit(1)
	}

	if os.Args[1] != "-v" {
		for {
			time.Sleep(60 * time.Second)
		}
	}
}
