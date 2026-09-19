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
- Configured Defaults support: a pack that has the mod keeps its config, and any other default files, in `configureddefaults/`
- Coloured output in the terminal: successes, warnings and errors are easy to tell apart, and updates show what changes

## Changelog and releases

A pack manages its own release history, from its git history. `packwiz git commit` turns changes to the pack into conventional commits, and a release is made from the commits since the last one: the changelog is what they say, and the version is what they add up to. (A pack that isn't in a git repository can still have a changelog, just [without the commits to read](#without-git).)

```
packwiz changelog          # preview the release the commits since the last one would make, including changes not committed yet
packwiz changelog release  # commit any changes, then record a release: bump pack.toml's version and update CHANGELOG.md
packwiz git commit         # commit each mod that was added, updated or removed on its own, then everything else
packwiz git release        # changelog release, then commit it as "chore(release): X.Y.Z" and tag it "vX.Y.Z"
```

So after adding, updating and removing mods, `packwiz changelog release` is all that's needed: it runs `packwiz git commit` first, so the log is up to date, then releases what is in it. The version is bumped by the most significant commit since the last release:

| Change | Version | Commit |
| --- | --- | --- |
| any change to a server or both-side mod | major | `feat(mods)!: add Lithium 0.12.0 (server)` |
| a client-only mod added or removed | minor | `feat(mods): add Sodium 0.5.7 (client)` |
| a client-only mod updated | patch | `fix(mods): update Iris 1.7.0 -> 1.7.1 (client)` |
| a config or other file changed | patch | `fix(config): change config/sodium.json` |
| anything else (pinning a mod, `pack.toml` edits, ...) | none | `chore(pack): update pack files` |

`packwiz git commit` makes a commit for each mod that changed, so after adding, updating and removing mods you only need to run it once:

```
feat(mods)!: add Lithium 0.12.0 (server)
feat(mods): remove Old Mod 1.0 (client)
fix(mods): update Iris 1.7.0 -> 1.7.1 (client)
fix(config): change config/sodium.json
```

Mods come first, in alphabetical order of their files, then one commit for everything else that changed: config files, a mod being pinned, edits to `pack.toml`. Every commit holds an `index.toml` and `pack.toml` that describe the pack as it is in that commit, not just as it ends up, so each one is a valid pack whose files match its index, and `git bisect` can be used to find the mod that broke something. Because mods go in alphabetical order, a mod can be committed before one it depends on. If a commit fails (a hook refused it, say), the ones before it stay made, and running the command again carries on with the rest. The first commit in a new repository is a single `chore(pack): initial commit` of the whole pack. `--dry-run` prints the commits it would make.

**How a release reads the log.** A release covers the commits made after the one the last release was made at, which `changelog.toml` records (`--since <commit, tag or branch>` reads from somewhere else instead). What `packwiz git commit` writes is read back as the change it describes. Several commits to the same mod are reduced to what they add up to, so a mod that was added and then updated is just added, and one added and then removed isn't mentioned, and doesn't make the release major. Other conventional commits count too, written by hand with plain git and shown in their own words: a `feat` is minor, a `fix` or `perf` is patch, and anything marked breaking (`feat!:` or a `BREAKING CHANGE:` footer) is major. Chores, documentation and commits that aren't conventional are left out.

```
fix(config): lower the particle count
feat!: update to Minecraft 1.21.4
```

The first release has no log to read, so it describes the pack as it is, and keeps the version already in `pack.toml`. Pass `--version X.Y.Z` to `release` to choose a higher version than the one worked out. The release history is kept in `changelog.toml` next to `pack.toml`, and `CHANGELOG.md` is generated from it; neither is distributed as part of the pack.

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
- lower the particle count

### Changes

- **Breaking:** update to Minecraft 1.21.4
```

Mods added before `version` was recorded have it looked up from Modrinth and saved to their `.pw.toml` the next time you run `packwiz changelog release` or `packwiz git commit` (`packwiz changelog` shows the versions but saves nothing), so that needs the network once. Releases made before then show file names, which are replaced with the versions too wherever the mod is still on that file.

### Without git

`packwiz git commit` and `packwiz git release` need the pack to be in a git repository, and refuse, before doing anything else, when it isn't. `packwiz changelog` and `packwiz changelog release` don't: they say that there is no git log to read, and make the changelog from the pack alone. The first release is the same as ever, and each release made this way keeps a snapshot of the pack in `changelog.toml`, so the next one lists what has changed in the pack since: mods added, updated and removed, and config files, with the version bumped by the same rules.

What comes from the log isn't there without one. Nothing is committed, so `packwiz changelog release` doesn't run `packwiz git commit` first (it saves the versions it looks up for mods that don't record one into their `.pw.toml` files itself, as committing would), there are no commits written by hand to list, and `--since` is an error. A pack that is put in a repository later is released from its log from then on. The log begins at its first commit, so anything that changed before that, since the last release, isn't in it. And a pack whose last release was made from the log can't be released without the repository, as nothing else says what the pack was like then.

## Configured Defaults

[Configured Defaults](https://modrinth.com/mod/configured-defaults) copies the files in a `configureddefaults` folder into the game directory when they are missing there, so a pack update doesn't overwrite what players changed. The folder can hold any file or folder of the game directory, not just `config/`.

A pack that has the mod added keeps all of its files there. `packwiz refresh` tracks only the mods' metadata files and what is in `configureddefaults/`, so the folder's files are what the index lists, what `packwiz serve` hands out, what `packwiz mr export` puts in the pack, and what `packwiz git commit` and the changelog describe as config:

```
fix(config): change configureddefaults/config/sodium.json
```

Files anywhere else, such as `config/` (which would replace players' own settings on every update), aren't tracked, and `.packwizignore` can't bring them back. The first `packwiz refresh` after adding the mod says how many files left the index; move them into `configureddefaults/` to keep them in the pack. Removing the mod makes every file trackable again.

## Coloured output

Output is coloured when it is going to a terminal: green for what was done, yellow for warnings, red for errors, cyan for notices, and bold or faded text to pick out names from file names and progress. `--help`, and the usage and error messages that come from getting a command wrong, are coloured too. It is never coloured when it is piped or redirected, so scripts, logs and files see the same plain text as always (colour is only added to the text, never changes it), and nothing that packwiz writes to your pack or to git has any in it. Each stream is decided on its own: help is written to stdout, so `packwiz --help > help.txt` is plain but `packwiz --help 2> errors.log` is still coloured, and the error and usage that come from getting a command wrong (`packwiz list extra`) are written to stderr, so `packwiz list extra 2> errors.log` has none in the log.

Use `--color` (or `--colour`) to choose when:

```
packwiz list --color=always   # colour even when piped, e.g. to a pager: packwiz update --all --color=always | less -R
packwiz list --color=never    # never
packwiz list --color=auto     # the default: colour a terminal
```

`PACKWIZ_COLOR` sets the same thing for every command, as does `color = "never"` in packwiz's config file (`.packwiz.toml` in packwiz's data folder, whose path `packwiz --help` shows under `--config`, or the file given with `--config`); the flag overrides both. With `auto`, the [`NO_COLOR`](https://no-color.org) environment variable turns colour off, as does `TERM=dumb`.

On Windows, colour is used in a console that can show it, such as Windows Terminal or a recent Windows 10 console host, and in the terminals of Git Bash (mintty), Cygwin and MSYS2, which aren't consoles but are recognised by the name of the pipe they give a program. If colour doesn't show up in one, ask for it with `--color=always` or `PACKWIZ_COLOR=always`.

## Installation
This fork does not currently publish prebuilt binaries, so install from source:

1. Install Go (1.24 or newer) from https://golang.org/dl/
2. Run `go install github.com/evictedcucumber/packwiz@latest`. Be patient, it has to download and compile dependencies as well!

Alternatively, if you use Nix, a flake is provided (see [flake.nix](flake.nix)).

## Documentation
See https://packwiz.infra.link/ for the full packwiz documentation!
