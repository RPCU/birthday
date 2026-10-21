package main

import (
	"os"

	"github.com/Banh-Canh/birthday/cmd"
)

func main() {
	os.Exit(cmd.Execute(os.Args[1:]))
}
