package main

import (
	// Modules of packwiz
	"github.com/evictedcucumber/packwiz/cmd"
	_ "github.com/evictedcucumber/packwiz/curseforge"
	_ "github.com/evictedcucumber/packwiz/migrate"
	_ "github.com/evictedcucumber/packwiz/settings"
	_ "github.com/evictedcucumber/packwiz/utils"
)

func main() {
	cmd.Execute()
}
