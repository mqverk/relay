package main

import (
	"os"

	"relay/internal/app"
)

func main() {
	os.Exit(app.Run(os.Args[1:]))
}
