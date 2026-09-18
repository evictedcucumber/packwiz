# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project overview

This is `github.com/evictedcucumber/packwiz`, a fork of [packwiz/packwiz](https://github.com/packwiz/packwiz), a CLI tool for creating Minecraft modpacks as version-controllable TOML metadata rather than raw JAR files. This fork only supports Modrinth as a mod source — CurseForge, GitHub releases, and direct-URL/local downloads have been removed from upstream. Packs created by this fork use a distinct `pack-format` (`evictedcucumber-packwiz:1.0.0`, see [core/pack.go](core/pack.go)) and are intentionally incompatible with upstream packwiz tooling.

## Commands

### Build

```bash
go build ./...
```

After any change, verify the project builds both ways CI and users build it:

```bash
go build ./...
nix build
```

`nix build` uses vendored dependencies pinned by `nix/vendor-hash`. If `go.mod`/`go.sum` changed, that hash must be refreshed: clear `nix/vendor-hash`, run `nix build`, and copy the `got:` hash from the failed build back into the file.

If `go` isn't on `PATH` (this repo doesn't assume a global Go install), run it through the flake dev shell instead: `nix develop --command go build ./...`.

### Test

```bash
go test ./...
```

Run a single package or test (mirrors what CI runs, `-race -count=2`, for a specific package):

```bash
go test ./core/... -race -count=2
go test ./modrinth/... -run TestSomeName -v
```

### Vet

```bash
go vet ./...
```

### Nix dev shell

```bash
nix develop
```

Provides the Go toolchain plus `gopls`, matching `flake.nix`. `direnv` (`.envrc` has `use flake`) picks this up automatically.

## Architecture

Commands are registered into a shared root via a plugin-style `init()` pattern, not a single monolithic command tree:

- [cmd/root.go](cmd/root.go) defines the root cobra command and global persistent flags (`--pack-file`, `--meta-folder`, `--meta-folder-base`, `--cache`, `--config`, `-y/--yes`), and exposes `cmd.Add()` for other packages to register subcommands.
- [cmd/](cmd) holds source-agnostic commands: `init`, `list`, `pin`/`unpin`, `refresh`, `remove`, `rehash`, `update`, `serve`, `mark-dependency`/`unmark-dependency`.
- [modrinth/](modrinth) implements the Modrinth source as a self-contained package: it registers its own `modrinth`/`mr` command (with `add [URL]`, `export`, `deps` subcommands) via `init()`, and implements `core.Updater` (registered into `core.Updaters["modrinth"]`) so the source-agnostic `packwiz update` can dispatch to it. [modrinth/api/](modrinth/api) is a thin REST client for the Modrinth API; the rest of [modrinth/](modrinth) contains install/export/dependency-resolution logic.
- [settings/](settings) (`acceptable-versions`, `release-type`), [migrate/](migrate) (`minecraft [version]`, `loader [version|latest|recommended]`), and [utils/](utils) (`markdown`) are similarly self-registering command packages, imported for side effects only (blank imports) from [main.go](main.go).
- [core/](core) has no dependency on `cmd` and holds the domain model shared by everything above: `Pack` ([core/pack.go](core/pack.go), the `pack.toml`), `Index`/`IndexFiles` ([core/index.go](core/index.go), [core/indexfiles.go](core/indexfiles.go), the `index.toml` manifest of tracked files), and `Mod` ([core/mod.go](core/mod.go), one TOML file per installed mod). [core/interfaces.go](core/interfaces.go) defines the `Updater` and `MetaDownloader` extension points that source packages implement.
- [cmdshared/](cmdshared) holds helpers shared across command packages (download progress/utilities, interactive prompts, Minecraft version lookups) that don't belong in `core`.

Because `core` doesn't import `cmd`, and source packages (`modrinth`) import both, new mod sources plug in the same way `modrinth` does: implement `core.Updater`/`core.MetaDownloader`, register in `init()`, and add a `cmd.Add()`-registered subcommand — without modifying `core` or `cmd`.

Configuration flows through `viper` globally (not passed explicitly through call chains): persistent flags are bound with `viper.BindPFlag` in `cmd/root.go`, and `Pack.Options` from `pack.toml` are merged into the same global viper config in `core.LoadPack` — so pack-level TOML options and CLI flags/env vars (`PACKWIZ_`-prefixed) are read through the same `viper.Get*` calls throughout the codebase.

## Testing conventions

Commands hold their logic directly in cobra `Run` closures that print via `fmt.Println` and exit via `os.Exit(1)` on error, rather than returning errors — so tests call `Run(cmd, args)` directly against an isolated temp directory and capture real stdout, instead of driving `rootCmd.Execute()`. [internal/cmdtest/cmdtest.go](internal/cmdtest/cmdtest.go) provides the shared helpers for this (`Chdir`, `CaptureStdout`, `WritePackFile`, `SetViper`/`SetViperBool`) and documents the viper state-leakage pitfalls between tests (see its doc comments, especially around `WritePackFile` and `Pack.Options` merging).

HTTP-calling code (Modrinth API client, downloads) is tested with `github.com/jarcoal/httpmock` rather than live network calls.
