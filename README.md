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
- Easy installation and updating of multiple mods at once from Modrinth
- Exporting to Modrinth packs, listing what goes in each one and which sides it is for
- `packwiz list --save` writes the names of a pack's mods, resource packs and shader packs to a markdown file, for a README or a pack page
- `packwiz validate` checks a pack's mods, dependencies and sides before you export it, and `packwiz fix` fixes what it can, showing the changes and asking first
- `packwiz config list` shows each mod's config files as a tree, with the files nothing claims shown at the end; `packwiz config relate` records which files a mod owns, or the mod loader (NeoForge's own config) or the pack as a whole (`options.txt`), and `packwiz validate` checks the same
- `packwiz`, run with no command, opens an interface in the terminal for the whole of packwiz: the pack's mods, adding projects from Modrinth (searched by name), updates, checking and fixing the pack, dependencies, config files, exporting, and releasing, each on a screen of its own that asks before it changes anything
- A mod can be named by a part of its name on the command line (`packwiz mr pin sdm`), searched for fuzzily the way fzf does, and the TUI's searches work the same way
- Server-only and Client-only mod handling
- Versioned releases with an automatic changelog, and git commits following conventional commits
- Configured Defaults support: a pack that has the mod keeps its config, and any other default files, in `configureddefaults/`
- Coloured output in the terminal: successes, warnings and errors are easy to tell apart, and updates show what changes

## Changelog and releases

A pack manages its own release history, from its git history. `packwiz git commit` turns changes to the pack into conventional commits, and a release is made from the commits since the last one: the changelog is what they say, and the version is what they add up to. (A pack that isn't in a git repository can still have a changelog, just [without the commits to read](#without-git).)

```
packwiz changelog          # preview the release the commits since the last one would make, including changes not committed yet
packwiz changelog --save   # write CHANGELOG.md from the releases already recorded, without releasing
packwiz changelog release  # record a release of what is committed: bump pack.toml's version and update CHANGELOG.md
packwiz git commit         # commit each mod and config file that changed on its own; files packwiz doesn't track are left alone
packwiz git release        # changelog release, then commit it as "chore(release): X.Y.Z" and tag it "vX.Y.Z"
```

So after adding, updating and removing mods, run `packwiz git commit` and then `packwiz changelog release`, which releases what is in the log. A release never commits anything itself: if the pack has changes that aren't committed it fails, saying to run `packwiz git commit` first. The version is bumped by the most significant commit since the last release:

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

Mods come first, in alphabetical order of their files, then each config file that was added, changed or removed, also on its own, and last one commit for what else changed in the pack's own files: a mod being pinned, edits to `pack.toml`. Files that packwiz doesn't recognise (anything in the pack's folder that is neither tracked by the index, nor `index.toml`, `pack.toml`, `changelog.toml`, `CHANGELOG.md`, `MODS.md` or `.packwizignore`, such as `README.md` or a file `.packwizignore` leaves out) are never committed: the command lists them and leaves them for you to commit with git, and `packwiz changelog release` refuses to release while there are any, since it can't say whether they belong in it. It refuses for a tracked config file that nothing claims too (the same thing `validate` fails on): those are committed, with a warning, but the pack is in a bad state until you claim them with `packwiz config relate` or delete them. Commit them yourself, or add them to `.gitignore`, and release again. Every commit holds an `index.toml` and `pack.toml` that describe the pack as it is in that commit, not just as it ends up, so each one is a valid pack whose files match its index, and `git bisect` can be used to find the mod that broke something. Because mods go in alphabetical order, a mod can be committed before one it depends on. If a commit fails (a hook refused it, say), the ones before it stay made, and running the command again carries on with the rest. The first commit in a new repository is a single `chore(pack): initial commit` of the whole pack. `--dry-run` prints the commits it would make.

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

Mods added before `version` was recorded have it looked up from Modrinth and saved to their `.pw.toml` the next time you run `packwiz git commit` (or, in a pack that isn't in a repository, `packwiz changelog release`) (`packwiz changelog` shows the versions but saves nothing), so that needs the network once. Releases made before then show file names, which are replaced with the versions too wherever the mod is still on that file.

### Without git

`packwiz git commit` and `packwiz git release` need the pack to be in a git repository, and refuse, before doing anything else, when it isn't. `packwiz changelog` and `packwiz changelog release` don't: they say that there is no git log to read, and make the changelog from the pack alone. The first release is the same as ever, and each release made this way keeps a snapshot of the pack in `changelog.toml`, so the next one lists what has changed in the pack since: mods added, updated and removed, and config files, with the version bumped by the same rules.

What comes from the log isn't there without one. Nothing is committed, so `packwiz changelog release` has no `packwiz git commit` to ask for first (it saves the versions it looks up for mods that don't record one into their `.pw.toml` files itself, as committing would), there are no commits written by hand to list, and `--since` is an error. A pack that is put in a repository later is released from its log from then on. The log begins at its first commit, so anything that changed before that, since the last release, isn't in it. And a pack whose last release was made from the log can't be released without the repository, as nothing else says what the pack was like then.

## Configured Defaults

[Configured Defaults](https://modrinth.com/mod/configured-defaults) copies the files in a `configureddefaults` folder into the game directory when they are missing there, so a pack update doesn't overwrite what players changed. The folder can hold any file or folder of the game directory, not just `config/`.

A pack that has the mod added keeps all of its files there. `packwiz refresh` tracks only the mods' metadata files and what is in `configureddefaults/`, so the folder's files are what the index lists, what `packwiz mr export` puts in the pack, and what `packwiz git commit` and the changelog describe as config:

```
fix(config): change configureddefaults/config/sodium.json
```

Files anywhere else, such as `config/` (which would replace players' own settings on every update), aren't tracked, and `.packwizignore` can't bring them back. The first `packwiz refresh` after adding the mod says how many files left the index; move them into `configureddefaults/` to keep them in the pack. Removing the mod makes every file trackable again.

## Listing a pack

`packwiz list` prints the names of what is in the pack: its mods, and any resource packs and shader packs. `packwiz list --save` writes them to `MODS.md`, next to `pack.toml`, to put in a README or a pack page:

```
# Cozy Adventures

A relaxed pack for exploring, building & farming

## Mods

- Architectury API
- Create: Steam 'n' Rails
- Sodium

## Resource Packs

- Faithful 32x

## Shader Packs

- Complementary Shaders - Reimagined
```

The names are in alphabetical order under a heading for each kind of file, going by the folder its `.pw.toml` is in. There are no versions, and nothing marks the mods that were added as dependencies; `--only main` leaves those out of the list instead, and `--side client` or `--side server` lists what is needed on one side, as they do for the plain list. `--output docs/mods.md` writes the file somewhere else (`--output -` prints it), and doesn't need `--save`. `MODS.md` isn't distributed with the pack, so if you choose another name inside the pack's folder, add it to `.packwizignore`.

## Config files

`packwiz config list` shows the pack's tracked files that aren't a mod's own metadata or destination file - normally what is in `config/` - as a tree of who each belongs to, with the files nothing claims under "Invalid" at the end. A config file belongs to a mod, to the pack's mod loader (NeoForge's own `config/neoforge-common.toml`, say) or to the pack as a whole (`options.txt`, which is Minecraft's and no mod's). The pack and the loader come first, then the mods:

```
Pack
└── options.txt

NeoForge
├── config/neoforge-client.toml
└── config/neoforge-common.toml

Sodium
└── config/sodium-options.json
Iris
└── config/iris/

Invalid
└── config/lithium.json
```

A mod records which files it owns in its `.pw.toml`'s `config-files`, a path, or a path ending in `/` to claim everything under it:

```
config-files = ["config/sodium-options.json", "config/iris/"]
```

The pack and its loader keep theirs in `pack.toml`, under `[config-files]`, written the same way and named `pack` or by the loader, as it is in `[versions]`:

```
[config-files]
pack = ["options.txt"]
neoforge = ["config/neoforge-client.toml", "config/neoforge-common.toml"]
```

All of this is written by hand, or with `packwiz config relate`; nothing derives it, so `options.txt` belongs to the pack once you say so, and is listed under "Invalid" until then. `--state valid` shows only what is claimed, and `--state invalid` only what isn't - the same as the "Invalid" section, for example a config file left behind by a mod that has since been removed, or never linked to the mod that installed it (as `--only main` does for `packwiz list`). `packwiz validate` reports the same files as a warning.

An entry that no tracked file matches - a config file that has since been deleted or renamed, or a folder with nothing left in it - is listed under its mod too, marked `(missing)`, and `--state missing` shows only those:

```
Sodium
├── config/sodium-options.json
└── config/sodium-old.json (missing)
```

`packwiz validate` warns about them for the mod, the pack or the loader that has them, and `packwiz fix` takes them out of its `config-files`. What is in the pack is what its index says, so after adding or deleting a config file run `packwiz refresh` first: until then the index still has the old files, and a file that is on disk but not yet in the index counts as missing.

Entries are always written as above, as if the pack kept its files at the root of the game directory. A pack that has [Configured Defaults](#configured-defaults) instead keeps everything in `configureddefaults/`, and `config-files` follows it there too: `config/sodium-options.json` claims `configureddefaults/config/sodium-options.json` once the mod is installed, with no need to rewrite it either way.

`packwiz config relate` adds a path to the config-files of whoever it is told: `--mod` for a mod, given the name of its `.pw.toml` file (as `packwiz mr pin` takes) and repeated for each mod, `--loader` for the pack's mod loader and `--pack` for the pack as a whole, in any combination, and a file or folder that exists in the pack:

```
packwiz config relate --mod sodium config/sodium-options.json
packwiz config relate --mod iris config/iris
packwiz config relate --pack options.txt
packwiz config relate --loader config/neoforge-common.toml config/neoforge-client.toml
```

A folder is given a trailing `/` automatically. If the pack has Configured Defaults, the path is written as if it didn't - the same normalizing `packwiz config list` does - so it reads the same whether or not the mod is installed. Running it again with the same arguments does nothing more. `--loader` fails for a pack that has no mod loader.

Every mod gets an empty `config-files = []` when it is added with `packwiz mr add`, and `packwiz fix` adds one to any mod that doesn't have one yet, so a `.pw.toml` file always shows the field is there to fill in with `packwiz config relate`.

## Naming a mod

The commands that take a mod - `packwiz mr pin`, `unpin`, `mark-dependency`, `unmark-dependency` and `update`, and `packwiz config relate --mod` - take it by its slug (the name of its `.pw.toml` file without the extension), that file name, or a path to it. If that is no mod's, what you typed is searched for in the names and slugs of the mods, fuzzily, the way fzf searches: its characters only have to be in order, and several words all have to be found, in any order, so `sdm` is Sodium and `sod ex` is Sodium Extra. Case doesn't matter.

```
$ packwiz mr pin sodex
sodex isn't the name of a mod, so using Sodium Extra (sodium-extra), which it matches
sodex pinned successfully!

$ packwiz mr pin sdm
"sdm" matches several mods, and it isn't clear which is meant: Sodium (sodium), Sodium Extra (sodium-extra); specify its slug, its .pw.toml file name, or a path to that file
```

What is searched for is only used if exactly one mod matches, and you are told which it was; if several do, none is chosen, as acting on the wrong mod is worse than asking again, and they are listed. A name that is exactly a mod's is always that mod, even if it is also part of the name of another. A path that isn't there is a mistake and isn't searched for. Nothing that worked before works differently: this only takes the place of the "Can't find" error.

## Terminal interface

Running `packwiz` with no command opens an interface for the pack in the current directory, in the terminal's alternate screen, so it leaves what was there when you quit. It is everything the commands do, on screens: go between them with `tab` and `shift+tab`, or press the number of one. In a folder that has no pack it opens on a screen that makes one, as `packwiz init` does, and carries on into the pack once it has.

| | Screen | What it is |
| --- | --- | --- |
| `1` | Overview | what the pack is and what is in it: its versions, how many mods and on which sides, the state of its config files |
| `2` | Mods | `packwiz list`, with pin (`p`), mark as dependency (`d`), update (`u`), the details of a mod (`enter`), search (`/`), filters for the side (`s`) and for main mods and dependencies (`m`), `w` to write `MODS.md` as `list --save` does, and `R` to refresh the index |
| `3` | Add | `packwiz modrinth add`: search Modrinth by name, or give the address of a project's page, or its slug or ID, and see what adding it would do and which mods it needs before you say yes |
| `4` | Updates | `packwiz update --all`: `c` looks for new versions, `space` picks which to update, `enter` updates what is picked |
| `5` | Check | `packwiz validate` and `packwiz fix`: what is wrong with the pack, by mod, and `f` to see what can be fixed, and make the changes |
| `6` | Deps | `packwiz modrinth deps`: what each mod needs and whether the pack has it, and `s` to save what had to be looked up |
| `7` | Config | `packwiz config list` and `packwiz config relate`, below |
| `8` | Export | `packwiz modrinth export`: where the pack goes and how, and what went into it |
| `9` | Release | `packwiz changelog`, `changelog release`, `git commit` and `git release`: the release the pack's changes would make, `C` to commit what isn't committed, `r` to release (and commit and tag it, in a repository; it won't start while changes aren't committed), `v` to choose the version, `s` to write `CHANGELOG.md` |

Every screen lists its keys at the bottom, and `?` lists them all. `j` and `k` move as the arrow keys do, `g` and `G` go to the top and the bottom, `/` searches (fuzzily, below), and `esc` leaves a search or a box. Keys that are letters are the screen's, so `tab` and the numbers don't change screens while text is being typed; `esc` leaves it first.

What it changes it changes as the commands do, through the same code, so what it writes is what they write and the index and `pack.toml` are left as a refresh would leave them. It asks before it changes anything that pressing a key again doesn't undo. It never writes to the terminal behind the screen: what the commands say as they go (that a version was used although another has a higher number, that a mod runs through Sinytra Connector, that a file failed to download) is shown on the screen it is for.

What asks the network (Modrinth, Mojang, the NeoForge repository) does so when you ask it to, apart from the first look at the Check, Deps and Release screens, and shows that it is working. One thing works on the pack at a time, so a screen that asked while another was busy waits its turn. `q` waits for what is changing the pack to finish, and `ctrl+c` quits at once. Colour follows `PACKWIZ_COLOR` and `NO_COLOR` like everything else, and everything selected or marked is shown with a symbol as well as a colour. It needs a terminal at least 40 columns wide and 10 lines high.

### Adding a project

`/` (or `i`) starts typing, and `enter` searches Modrinth for mods that suit the pack's game version and mod loader (and Fabric mods too, for a pack that runs them through Sinytra Connector). The results are listed with their author and how often they were downloaded, and the ones the pack has are marked. `enter` again adds the one under the cursor (or searches again if you changed what you typed); an address such as `https://modrinth.com/mod/sodium` goes straight to it. `tab` while typing, or `t` after, searches for resource packs and then shaders instead, and `ctrl+r` (or `r`) says how unstable a version can be and still be taken, as `--release-type` does.

Before anything is added it says what would be: the version and file, where it goes, which side it is for, what it needs that the pack doesn't have, and notices such as that it is a Fabric mod. `y` adds it and what it needs, `d` adds it without what it needs, and `n` leaves the pack alone. A project the pack has already is offered an update in place, and a pinned one is refused, as with the command.

### Config files

The config screen is the tree that `packwiz config list` prints, which you move through instead of reading, and change while you are there.

| Key | |
| --- | --- |
| `↑` `↓` (or `k` `j`), `g` `G`, page up and down | move through the tree |
| `←` `→` (or `h` `l`), `enter` on a mod | fold and unfold what a mod owns; `←` from a file goes to its mod |
| `r` (or `enter` on a file) | relate the file to the pack, its mod loader or mods |
| `x` | take an owner's claim on a file out, or an entry that matches no file |
| `space`, `esc` | mark a file, to relate several together (on a mod or "Invalid", every file in it); `esc` clears a search that is left on, and then unmarks everything |
| `/` | search the tree, fuzzily: only the files that match are shown, and the cursor goes to the best match |
| `f` | show all files, then only the valid, the invalid and the missing ones, as `--state` does |
| `R` | refresh the index, so files added or deleted since show up |
| `?`, `q` | all of the keys, and quit |

`/` searches the files as `r` searches the mods, below: type to narrow the tree to what matches, the arrow keys move through it while you type, `enter` leaves the search on and makes the keys commands again, and `esc` clears it. Each word has to be in a file's path or in the name of what owns it, so `sodium json` finds Sodium's json files and `sodium` all of its files; a group that was folded is shown in full while you search, and what `space` marks on a group is only what matched. The letters that matched are picked out.

`r` asks who to give the file to, the pack and the loader first ("Pack" is the pack as a whole, for `options.txt` and the like; the loader is NeoForge) and then the mods: type `/` to search them, `space` picks one, and `enter` relates the file to what is picked, or to the mod under the cursor if none is. The search is fuzzy, the way fzf's is, and nothing has to be installed for it: `sdm` finds Sodium, letters only have to be in order, several words (`sod ex`) all have to match in any order, case doesn't matter, and the mod's slug is searched too when its name doesn't match. The best match is first, so `enter` takes it, and the letters that matched are picked out. Several can be picked for a file they share. `tab` switches between claiming the file and claiming the folder it is in, and what would be written to each one's `config-files` is shown above the list. Marked files are related together, and an owner that already claims everything being related is labelled as such. `x` shows the entry that would be taken out, and how many files it covers if it is a folder, and asks before doing it.

It writes what `packwiz config relate` writes, through the same code, so the `.pw.toml` files, the index and `pack.toml` (where the pack's and the loader's claims go) are left as if the commands had made the changes, and a pack that keeps its files in `configureddefaults/` has its entries written without it, as they are there.

## Checking a pack

`packwiz mr export` lists the files it put in the pack once they are downloaded, with the sides each is needed on (`required`, `optional` or `unsupported` on the client and on the server, which is what the launcher goes by), its size, and where it goes:

```
Exported files:
Mod                                     Client       Server        Size  File
Farmer's Delight                        required     required   3.0 MiB  mods/FarmersDelight-1.21.1-1.3.4.jar
Lib Mod                                 unsupported  required  19.5 KiB  mods/lib.jar
Repurposed Structures - Neoforge/Forge  required     required   8.0 MiB  mods/repurposed_structures-7.5.22+1.21.1-neoforge.jar
3 files, 11.0 MiB: 2 on both sides, 1 server only
```

`packwiz validate` checks the pack, without changing it, for what would go wrong once it is exported or played, and fails (exit status 1) if it finds an error, so it can be used in a script:

- every mod's `.pw.toml` can be read and has what it needs: a name, file name, download URL and hash, a side, and the Modrinth project and version it came from (some things a pack can do without, like a name or a recorded version, are warnings, which don't fail it)
- no two mods are the same Modrinth project, or install to the same file
- every required dependency is in the pack, and nothing in it is incompatible with something else
- every mod that a mod on the client requires is on the client too: Modrinth lists some libraries, mostly for world generation, as unsupported on the client, but a mod that requires one crashes the game without it. `packwiz mr add` puts such a mod on both sides, and `validate` (and a warning from `export`) finds one that isn't
- `pack.toml` has a Minecraft version, a NeoForge version and a version for the pack
- every tracked config file is claimed by a mod's `config-files`, or by the pack's or its loader's in `pack.toml` (see [Config files](#config-files) and `packwiz config list --state invalid`), which is an error, not a warning, as the pack can't be released with one; and every entry in any of them matches a file in the pack (`packwiz config list --state missing`, a warning); `pack.toml`'s `[config-files]` names only `pack` and a loader the pack has

What a mod depends on is what its `.pw.toml` records. A mod that records nothing is looked up on Modrinth, so that needs the network; if it can't be reached, those mods' dependencies aren't checked, and it says so, but the rest of the checks are made.

`packwiz fix` runs `validate`, works out what can be done about what it finds, shows the changes, and asks before making any (the changes are shown first). Afterwards it checks the pack again, and fails if errors are left:

```
Changes to make:
Repurposed Structures - Neoforge/Forge (mods/repurposed-structures-forge.pw.toml):
  add: version 7.5.22+1.21.1-neoforge (repurposed_structures-7.5.22+1.21.1-neoforge.jar), which "Repurposed Structures - Farmer's Delight Compat" requires
  side: server -> both, as "Repurposed Structures - Farmer's Delight Compat" needs it on the client

Would you like to make these changes? [Y/n]:
```

- a mod that is only on the server, but that a mod on the client requires, is put on both sides
- a required dependency that isn't in the pack is added, at its latest version, as `packwiz mr add` would add it, and put on both sides if a mod on the client requires it (one that can't be found a version of, or whose file name is already taken, is left and reported)
- a mod with no side is given one (`both`, which it was treated as having), and a mod that doesn't record its version has it recorded
- a mod with no `config-files` gets an empty one, ready for `packwiz config relate` to fill in, and an entry in a mod's `config-files`, or in the pack's or its loader's in `pack.toml`, that matches no file in the pack is taken out (run `packwiz refresh` first, as a file that isn't in the index yet counts as missing)

Everything else `validate` finds, such as a mod that is incompatible with another, needs a decision, so it is left for you.

## Coloured output

Output is coloured when it is going to a terminal: green for what was done, yellow for warnings, red for errors, cyan for notices, and bold or faded text to pick out names from file names and progress. `--help`, and the usage and error messages that come from getting a command wrong, are coloured too. It is never coloured when it is piped or redirected, so scripts, logs and files see the same plain text as always (colour is only added to the text, never changes it), and nothing that packwiz writes to your pack or to git has any in it. Each stream is decided on its own: help is written to stdout, so `packwiz --help > help.txt` is plain but `packwiz --help 2> errors.log` is still coloured, and the error and usage that come from getting a command wrong (`packwiz list extra`) are written to stderr, so `packwiz list extra 2> errors.log` has none in the log.

Set `PACKWIZ_COLOR` to choose when, for every command: `always` colours even when piped (e.g. to a pager: `PACKWIZ_COLOR=always packwiz mr update --all | less -R`), `never` never does, and `auto` (the default) colours a terminal. `color = "never"` in packwiz's config file (`.packwiz.toml` in packwiz's data folder) does the same, and the environment variable overrides it. With `auto`, the [`NO_COLOR`](https://no-color.org) environment variable turns colour off, as does `TERM=dumb`.

On Windows, colour is used in a console that can show it, such as Windows Terminal or a recent Windows 10 console host, and in the terminals of Git Bash (mintty), Cygwin and MSYS2, which aren't consoles but are recognised by the name of the pipe they give a program. If colour doesn't show up in one, ask for it with `PACKWIZ_COLOR=always`.

## Installation
This fork does not currently publish prebuilt binaries, so install from source:

1. Install Go (1.24 or newer) from https://golang.org/dl/
2. Run `go install github.com/evictedcucumber/packwiz@latest`. Be patient, it has to download and compile dependencies as well!

Alternatively, if you use Nix, a flake is provided (see [flake.nix](flake.nix)).

## Documentation
See https://packwiz.infra.link/ for the full packwiz documentation!
