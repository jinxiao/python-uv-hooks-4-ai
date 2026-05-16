package main

import (
	"os"

	"uv-python-hook/internal/hook"
)

func main() {
	os.Exit(hook.Run(os.Args[1:]))
}
