# Configuration Reference

## pack.toml

A minimal `pack.toml` with the new fields looks like this:

```toml
name = "Example Pack"
author = "Example Author"
version = "1.0.0"
pack-format = "packwiz:1.1.0"
loader = "modrinth"
modlist = true

[index]
file = "index.toml"

[versions]
minecraft = "1.21.1"
fabric = "0.16.9"
```

### Field Notes

- `loader` is required and must be `modrinth` or `curseforge`.
- `modlist` controls whether `modlist.md` is generated and maintained automatically.
- `versions.minecraft` is required.
- `index.file` points at the pack index file.

## Mod Manifest Example

A mod manifest now carries the page URL and a human-readable version string used by `modlist.md`:

```toml
name = "Example Mod"
filename = "example-mod-1.2.3.jar"
version = "1.2.3"
page-url = "https://modrinth.com/mod/example-mod"
category = "mods"

[download]
url = "https://cdn.modrinth.com/data/.../example-mod-1.2.3.jar"
hash-format = "sha256"
hash = "..."
mode = "url"

[update.modrinth]
mod-id = "example-mod"
version = "abc123"
```

### Validation Rules

- `page-url` must be present for entries that will be included in `modlist.md`.
- `version` must be present for entries that will be included in `modlist.md`.
- `packwiz validate` reports missing values and tells you which manifest needs fixing.
