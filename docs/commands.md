# Command Reference

## Init

Create a new pack and choose the loader explicitly.

```bash
packwiz init --loader curseforge --modloader forge --mc-version 1.20.1
```

Useful flags:

- `--loader modrinth|curseforge`
- `--modlist`
- `--latest`
- `--snapshot`

## Add

Add source-backed content by URL only.

```bash
packwiz mr add https://modrinth.com/mod/sodium
packwiz cf add https://www.curseforge.com/minecraft/mc-mods/sodium
```

Modrinth shaderpack URLs are supported and install into `shaderpacks/` when applicable.

## Update

Update a single mod by slug or by `.pw.toml` path.

```bash
packwiz update sodium
packwiz update mods/sodium.pw.toml
```

The `mr` and `cf` command groups also expose their own `update` and `add` subcommands for source-specific workflows.

Update all mods:

```bash
packwiz update --all
```

## Validate

Check the pack and metadata for configuration mistakes and missing modlist data.

```bash
packwiz validate
```

This checks for things like:

- missing `loader`
- missing `modlist`
- missing `index.file`
- missing `page-url` or `version` in mod manifests

## Modlist

Generate or repair `modlist.md`.

```bash
packwiz modlist generate
packwiz modlist validate
packwiz modlist fix
```

The generated list is grouped by category and sorted alphabetically by mod name inside each group.
