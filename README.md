# packwiz

packwiz is a command line tool for creating Minecraft modpacks. It stores pack state in TOML metadata files, which keeps packs easy to version control, inspect, and share.

## Quick Links

- [Getting Started](docs/index.md)
- [Configuration Reference](docs/configuration.md)
- [Command Reference](docs/commands.md)
- [Modlist Guide](docs/modlist.md)

## Current Workflow

- `packwiz init` creates a `pack.toml` with an explicit `loader = "modrinth"` or `loader = "curseforge"` choice.
- `packwiz mr add` and `packwiz cf add` accept project URLs only for Modrinth and CurseForge sources.
- `packwiz validate` checks `pack.toml` and mod metadata, including missing `page-url` and `version` fields needed by `modlist.md`.
- `packwiz modlist` generates, validates, and fixes `modlist.md` using source page URLs.

## Installation

You can also compile from source with Go 1.24 or newer:

```bash
go install github.com/evictedcucumber/packwiz@latest
```

## Notes

- The command-line documentation in this repository is the source of truth for the current feature set.

## Credits

This fork is based on the original packwiz project and its contributors: https://github.com/packwiz/packwiz
The `LICENSE` file in this repository includes both the original and new licensing terms: changes from the original packwiz remain under the original license, while new changes in this fork are covered by the new license.
