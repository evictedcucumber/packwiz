# Modlist Guide

`modlist.md` is a generated overview of the pack's mods.

## Format

Each entry is rendered as:

```md
- [Name - (Version)](Page URL)
```

Entries are grouped by category, such as:

- `mods`
- `resourcepacks`
- `shaderpacks`

Within each category, entries are sorted alphabetically by mod name.

## Required Metadata

For `modlist.md` to be generated cleanly, each manifest needs:

- `name`
- `version`
- `page-url`

If any of those are missing, `packwiz validate` reports the exact manifest that needs attention.

## Commands

```bash
packwiz modlist generate
packwiz modlist validate
packwiz modlist fix
```

## Example Output

```md
# Mods List

## Mods

- [Sodium - (0.6.0)](https://modrinth.com/mod/sodium)
- [Lithium - (0.13.1)](https://modrinth.com/mod/lithium)

## Resourcepacks

- [Faithful - (32x)](https://modrinth.com/resourcepack/faithful)
```
