# packwiz

This is a fork of [packwiz/packwiz](https://github.com/packwiz/packwiz), the original packwiz project created and maintained by [comp500](https://github.com/comp500) and contributors. All credit for the original design and implementation goes to the upstream project — see their repository for the canonical version.

packwiz is a command line tool for creating Minecraft modpacks. Instead of managing JAR files directly, packwiz creates TOML metadata files which can be easily version-controlled and shared with git (see an example pack [here](https://github.com/packwiz/packwiz-example-pack)). You can then [export it to a Modrinth modpack](https://packwiz.infra.link/tutorials/hosting/modrinth/), or [use packwiz-installer](https://packwiz.infra.link/tutorials/installing/packwiz-installer/) for an auto-updating MultiMC instance.

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
- Versioned releases with an automatic changelog, and git commits following conventional commits

## Changelog and releases

A pack manages its own release history. Each mod's `.pw.toml` records its readable `version` when it is added or updated, and a release remembers what the pack contained, so the next release can describe exactly what changed.

```
packwiz changelog          # preview what changed since the last release, and the version it would make
packwiz changelog release  # bump pack.toml's version, update CHANGELOG.md, and remember the pack's contents
packwiz git commit         # commit every change to the pack with a generated conventional commit message
packwiz git release        # changelog release, then commit it as "chore(release): X.Y.Z" and tag it "vX.Y.Z"
```

The version is bumped by the most significant change since the last release:

| Change | Version | Commit |
| --- | --- | --- |
| any change to a server or both-side mod | major | `feat(mods)!: add Lithium 0.12.0 (server)` |
| a client-only mod added or removed | minor | `feat(mods): add Sodium 0.5.7 (client)` |
| a client-only mod updated | patch | `fix(mods): update Iris 1.7.0 -> 1.7.1 (client)` |
| a config or other file changed | patch | `fix(config): change config/sodium.json` |
| anything else (pinning a mod, `pack.toml` edits, ...) | none | `chore(pack): update pack files` |

The first release keeps the version already in `pack.toml`. Pass `--version X.Y.Z` to `release` to choose a higher version than the one worked out. The release history is kept in `changelog.toml` next to `pack.toml`, and `CHANGELOG.md` is generated from it; neither is distributed as part of the pack. `packwiz git release` needs everything committed first (`packwiz git commit`), so the release commit contains only the release.

`CHANGELOG.md` lists what each release added, updated and removed, with what runs on the server first, and asks for a server update when the release is major:

```markdown
## 2.0.0 - 2026-09-18

> **Server update required.** This release changes mods that run on the server.

### Added

- **Lithium** 0.12.0 (server)

### Updated

- **Fabric API** 0.100 → 0.101 (client + server)
- **Iris** 1.7.0 → 1.7.1 (client)

### Config

- Changed `config/sodium.json`
```

Mods added before `version` was recorded have it looked up from Modrinth and saved to their `.pw.toml` the next time you run `packwiz changelog release` or `packwiz git commit` (`packwiz changelog` shows the versions but saves nothing), so that needs the network once. Releases made before then show file names, which are replaced with the versions too wherever the mod is still on that file.

## Installation
This fork does not currently publish prebuilt binaries, so install from source:

1. Install Go (1.24 or newer) from https://golang.org/dl/
2. Run `go install github.com/evictedcucumber/packwiz@latest`. Be patient, it has to download and compile dependencies as well!

Alternatively, if you use Nix, a flake is provided (see [flake.nix](flake.nix)).

## Documentation
See https://packwiz.infra.link/ for the full packwiz documentation!
