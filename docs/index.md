# Getting Started

packwiz keeps a modpack in TOML files instead of managing loose JARs directly.

## Typical Flow

1. Create a pack with `packwiz init`.
2. Add content from Modrinth or CurseForge using a URL.
3. Run `packwiz refresh` when you add or remove files manually.
4. Run `packwiz validate` before sharing a pack.
5. Use `packwiz modlist generate` if you want to regenerate the mod list file.

## Example Init

```bash
packwiz init --loader modrinth --modloader fabric --mc-version 1.21.1 --name "Example Pack" --modlist
```
