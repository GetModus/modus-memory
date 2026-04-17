package main

import (
	"os"

	"github.com/GetModus/modus-memory/internal/app"
)

func main() {
	app.Main("homing", os.Args[1:])
}
