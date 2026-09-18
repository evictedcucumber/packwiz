package main

import (
	// Modules of packwiz
	_ "github.com/evictedcucumber/packwiz/changelog"
	"github.com/evictedcucumber/packwiz/cmd"
	_ "github.com/evictedcucumber/packwiz/migrate"
	_ "github.com/evictedcucumber/packwiz/modrinth"
	_ "github.com/evictedcucumber/packwiz/settings"
	_ "github.com/evictedcucumber/packwiz/utils"
)

func main() {
	cmd.Execute()
}
