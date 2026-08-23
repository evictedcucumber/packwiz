package main

import (
	// Modules of packwiz
	"github.com/evictedcucumber/packwiz/cmd"
	_ "github.com/evictedcucumber/packwiz/curseforge"
	_ "github.com/evictedcucumber/packwiz/github"
	_ "github.com/evictedcucumber/packwiz/migrate"
	_ "github.com/evictedcucumber/packwiz/modrinth"
	_ "github.com/evictedcucumber/packwiz/settings"
	_ "github.com/evictedcucumber/packwiz/url"
	_ "github.com/evictedcucumber/packwiz/utils"
)

func main() {
	cmd.Execute()
}
