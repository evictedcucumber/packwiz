package main

import (
	// Modules of packwiz
	_ "github.com/evictedcucumber/packwiz/changelog"
	"github.com/evictedcucumber/packwiz/cmd"
	_ "github.com/evictedcucumber/packwiz/git"
	_ "github.com/evictedcucumber/packwiz/modrinth"
	_ "github.com/evictedcucumber/packwiz/tui"
	_ "github.com/evictedcucumber/packwiz/utils"
)

func main() {
	cmd.Execute()
}
