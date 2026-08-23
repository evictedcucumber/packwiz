# packwiz

This is a fork of [packwiz/packwiz](https://github.com/packwiz/packwiz), the original packwiz project created and maintained by [comp500](https://github.com/comp500) and contributors. All credit for the original design and implementation goes to the upstream project — see their repository for the canonical version.

packwiz is a command line tool for creating Minecraft modpacks. Instead of managing JAR files directly, packwiz creates TOML metadata files which can be easily version-controlled and shared with git (see an example pack [here](https://github.com/packwiz/packwiz-example-pack)). You can then [export it to a Modrinth modpack](https://packwiz.infra.link/tutorials/hosting/curseforge/), or [use packwiz-installer](https://packwiz.infra.link/tutorials/installing/packwiz-installer/) for an auto-updating MultiMC instance.

This fork only supports Modrinth as a mod source - CurseForge, GitHub releases, and direct-URL/local downloads have been removed.

packwiz is great for...

- Distributing private modpacks for servers
- Creating modpacks for Modrinth

packwiz is not so great for...

- Managing downloaded mod files (use [Curse/GDLauncher or another CLI](https://gist.github.com/comp500/13ae6f058221196077fb19953ac608c7))
- People who want a GUI (though there are some [third-party efforts](https://github.com/ExoPlant/packwiz-gui))

Join the upstream packwiz Discord server if you need help [here](https://discord.gg/Csh8zbbhCt)!

## Features
- Git-friendly TOML-based metadata format
- MultiMC pack installer/updater, with support for optional mods and fast automatic updates - perfect for servers!
- Pack distribution with HTTP servers, with a built in local server for testing
- Easy installation and updating of multiple mods at once from Modrinth
- Exporting to Modrinth packs
- Server-only and Client-only mod handling

## Installation
Prebuilt binaries are available from [GitHub Actions](https://github.com/evictedcucumber/packwiz/actions) - the UI is a bit terrible, but essentially select the top build, then download the artifact ZIP for your system at the bottom of the page.  

Another option is to use [nightly.link](https://nightly.link/evictedcucumber/packwiz/workflows/go/main). Just go to the page, and download the artifact for your system.  

To run the executable, first extract it, then add the folder where you extracted it to your PATH environment variable ([see tutorial for Windows here](https://www.howtogeek.com/118594/how-to-edit-your-system-path-for-easy-command-line-access/)) or move it to where you want to use it.

You can also compile from source:

1. Install Go (1.24 or newer) from https://golang.org/dl/
2. Run `go install github.com/evictedcucumber/packwiz@latest`. Be patient, it has to download and compile dependencies as well!

## Documentation
See https://packwiz.infra.link/ for the full packwiz documentation!
