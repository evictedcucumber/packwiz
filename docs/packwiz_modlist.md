## packwiz modlist

Generate, validate, or fix modlist.md

### Options

```
  -h, --help   help for modlist
```

### Options inherited from parent commands

```
      --cache string              The directory where packwiz will cache downloaded mods (default "/home/ethan/.local/cache/packwiz/cache")
      --config string             The config file to use (default "/home/ethan/.local/share/packwiz/.packwiz.toml")
      --meta-folder string        The folder in which new metadata files will be added, defaulting to a folder based on the category (mods, resourcepacks, etc; if the category is unknown the current directory is used)
      --meta-folder-base string   The base folder from which meta-folder will be resolved, defaulting to the current directory (so you can put all mods/etc in a subfolder while still using the default behaviour) (default ".")
      --pack-file string          The modpack metadata file to use (default "pack.toml")
  -y, --yes                       Accept all prompts with the default or "yes" option (non-interactive mode) - may pick unwanted options in search results
```

### SEE ALSO

* [packwiz](packwiz.md)	 - A command line tool for creating Minecraft modpacks
* [packwiz modlist fix](packwiz_modlist_fix.md)	 - Rewrite modlist.md to match the current pack
* [packwiz modlist generate](packwiz_modlist_generate.md)	 - Generate or overwrite modlist.md
* [packwiz modlist validate](packwiz_modlist_validate.md)	 - Check whether modlist.md matches the current pack

