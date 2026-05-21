package main

import (
	"flag"
	"os"

	"github.com/fatih/color"
)

var green = color.New(color.FgGreen).Add(color.Bold)

func main() {
	var iconsPath string
	flag.StringVar(&iconsPath, "icons", "", "the path to place icons.json at")
	flag.Parse()

	status := generateIconList(iconsPath)
	os.Exit(status)
}
