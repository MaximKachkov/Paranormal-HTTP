package main

import (
	"os"

	"paranormal-http/internal/app"
)

func main() {
	os.Exit(app.Main(os.Args[1:]))
}
