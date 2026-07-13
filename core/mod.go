package core

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/spf13/viper"
)

// Mod stores metadata about a mod. This is written to a TOML file for each mod.
type Mod struct {
	metaFile string      // The file for the metadata file, used as an ID
	Name     string      `toml:"name"`
	FileName string      `toml:"filename"`
	Version  string      `toml:"version,omitempty"`
	PageURL  string      `toml:"page-url,omitempty"`
	Category string      `toml:"category,omitempty"`
	Side     string      `toml:"side,omitempty"`
	Pin      bool        `toml:"pin,omitempty"`
	Download ModDownload `toml:"download"`
	// Update is a map of map of stuff, so you can store arbitrary values on string keys to define updating
	Update     map[string]map[string]interface{} `toml:"update"`
	updateData map[string]interface{}

	// Dependencies stores references to other mod metadata files this mod depends on.
	Dependencies []string `toml:"dependencies,omitempty"`

	UpdateChannel string `toml:"update-channel,omitempty"`

	Option *ModOption `toml:"option,omitempty"`
}

const (
	ModeURL string = "url"
	ModeCF  string = "metadata:curseforge"
)

// ModDownload specifies how to download the mod file
type ModDownload struct {
	URL        string `toml:"url,omitempty"`
	HashFormat string `toml:"hash-format"`
	Hash       string `toml:"hash"`
	// Mode defaults to modeURL (i.e. use URL when omitted or empty)
	Mode string `toml:"mode,omitempty"`
}

// ModOption specifies optional metadata for this mod file
type ModOption struct {
	Dependency  bool   `toml:"dependency"`
	Description string `toml:"description,omitempty"`
}

// EnsureOptionDefaults ensures every mod has an explicit option table with dependency set.
func (m *Mod) EnsureOptionDefaults() {
	if m.Option == nil {
		m.Option = &ModOption{Dependency: false}
	}
}

// The four possible values of Side (the side that the mod is on) are "server", "client", "both", and "" (equivalent to "both")
const (
	ServerSide    = "server"
	ClientSide    = "client"
	UniversalSide = "both"
	EmptySide     = ""
)

// LoadMod attempts to load a mod file from a path
func LoadMod(modFile string) (Mod, error) {
	var mod Mod
	if _, err := toml.DecodeFile(modFile, &mod); err != nil {
		return Mod{}, err
	}
	mod.updateData = make(map[string]interface{})
	// Horrible reflection library to convert map[string]interface to proper struct
	for k, v := range mod.Update {
		updater, ok := Updaters[k]
		if ok {
			updateData, err := updater.ParseUpdate(v)
			if err != nil {
				return mod, err
			}
			mod.updateData[k] = updateData
		} else {
			return mod, errors.New("Update plugin " + k + " not found!")
		}
	}
	// Store path relative to pack root if possible
	packFile := viper.GetString("pack-file")
	if packFile != "" {
		packDir := filepath.Dir(packFile)
		if filepath.IsAbs(modFile) {
			rel, err := filepath.Rel(packDir, modFile)
			if err == nil {
				modFile = filepath.ToSlash(rel)
			}
		} else {
			// Already relative, ensure it's in forward slash format
			modFile = filepath.ToSlash(modFile)
		}
	}
	mod.metaFile = modFile
	return mod, nil
}

// SetMetaPath sets the file path of a metadata file (stored as relative to pack root)
func (m *Mod) SetMetaPath(metaFile string) string {
	// Convert to relative path from pack root if possible
	packFile := viper.GetString("pack-file")
	if packFile != "" {
		packDir := filepath.Dir(packFile)
		if filepath.IsAbs(metaFile) {
			rel, err := filepath.Rel(packDir, metaFile)
			if err == nil {
				metaFile = filepath.ToSlash(rel)
			}
		} else {
			// Already relative, ensure it's in forward slash format
			metaFile = filepath.ToSlash(metaFile)
		}
	}
	m.metaFile = metaFile
	return m.metaFile
}

// resolveMetaPath resolves the stored metaFile path to an absolute path for file operations
func (m *Mod) resolveMetaPath() string {
	if filepath.IsAbs(m.metaFile) {
		return m.metaFile
	}
	packFile := viper.GetString("pack-file")
	if packFile == "" {
		packFile = "pack.toml"
	}
	packDir := filepath.Dir(packFile)
	return filepath.Join(packDir, filepath.FromSlash(m.metaFile))
}

// Write saves the mod file, returning a hash format and the value of the hash of the saved file
func (m Mod) Write() (string, string, error) {
	metaPath := m.resolveMetaPath()
	f, err := os.Create(metaPath)
	if err != nil {
		// Attempt to create the containing directory
		err2 := os.MkdirAll(filepath.Dir(metaPath), os.ModePerm)
		if err2 == nil {
			f, err = os.Create(metaPath)
		}
		if err != nil {
			return "sha256", "", err
		}
	}

	h, err := GetHashImpl("sha256")
	if err != nil {
		_ = f.Close()
		return "", "", err
	}
	w := io.MultiWriter(h, f)

	enc := toml.NewEncoder(w)
	// Disable indentation
	enc.Indent = ""
	err = enc.Encode(m)
	hashString := h.HashToString(h.Sum(nil))
	if err != nil {
		_ = f.Close()
		return "sha256", hashString, err
	}
	return "sha256", hashString, f.Close()
}

// GetParsedUpdateData can be used to retrieve updater-specific information after parsing a mod file
func (m Mod) GetParsedUpdateData(updaterName string) (interface{}, bool) {
	upd, ok := m.updateData[updaterName]
	return upd, ok
}

// GetFilePath is a clumsy hack that I made because Mod already stores it's path anyway
func (m Mod) GetFilePath() string {
	return m.resolveMetaPath()
}

// GetMetaPath returns the stored relative path to the metadata file
func (m Mod) GetMetaPath() string {
	return m.metaFile
}

// GetDestFilePath returns the path of the destination file of the mod
func (m Mod) GetDestFilePath() string {
	return filepath.Join(filepath.Dir(m.resolveMetaPath()), filepath.FromSlash(m.FileName))
}

var slugifyRegex1 = regexp.MustCompile(`\(.*\)`)
var slugifyRegex2 = regexp.MustCompile(` - .+`)
var slugifyRegex3 = regexp.MustCompile(`[^a-z\d]`)
var slugifyRegex4 = regexp.MustCompile(`-+`)
var slugifyRegex5 = regexp.MustCompile(`^-|-$`)

func SlugifyName(name string) string {
	lower := strings.ToLower(name)
	noBrackets := slugifyRegex1.ReplaceAllString(lower, "")
	noSuffix := slugifyRegex2.ReplaceAllString(noBrackets, "")
	limitedChars := slugifyRegex3.ReplaceAllString(noSuffix, "-")
	noDuplicateDashes := slugifyRegex4.ReplaceAllString(limitedChars, "-")
	noLeadingTrailingDashes := slugifyRegex5.ReplaceAllString(noDuplicateDashes, "")
	return noLeadingTrailingDashes
}
