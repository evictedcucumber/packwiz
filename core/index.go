package core

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/evictedcucumber/packwiz/internal/fuzzy"
	"github.com/evictedcucumber/packwiz/internal/ui"
	gitignore "github.com/sabhiram/go-gitignore"
	"github.com/spf13/viper"
	"github.com/vbauerster/mpb/v4"
	"github.com/vbauerster/mpb/v4/decor"
)

// Index is a representation of the index.toml file for referencing all the files in a pack.
type Index struct {
	HashFormat string
	Files      IndexFiles
	indexFile  string
	packRoot   string
}

// indexTomlRepresentation is the TOML representation of Index (Files must be converted)
type indexTomlRepresentation struct {
	HashFormat string                       `toml:"hash-format"`
	Files      indexFilesTomlRepresentation `toml:"files"`
}

// LoadIndex attempts to load the index file from a path
func LoadIndex(indexFile string) (Index, error) {
	data, err := os.ReadFile(indexFile)
	if err != nil {
		return Index{}, err
	}
	index, err := ParseIndex(data)
	if err != nil {
		return Index{}, err
	}
	index.indexFile = indexFile
	index.packRoot = filepath.Dir(indexFile)
	return index, nil
}

// ParseIndex parses the contents of an index file that isn't necessarily on disk (e.g. a committed version of it).
// The Index it returns lists its files, but has no location, so it can't resolve them to paths on disk or be written.
func ParseIndex(data []byte) (Index, error) {
	// Decode as indexTomlRepresentation then convert to Index
	var rep indexTomlRepresentation
	if _, err := toml.Decode(string(data), &rep); err != nil {
		return Index{}, err
	}
	if len(rep.HashFormat) == 0 {
		rep.HashFormat = "sha256"
	}
	return Index{
		HashFormat: rep.HashFormat,
		Files:      rep.Files.toMemoryRep(),
	}, nil
}

// RemoveFile removes a file from the index, given a file path
func (in *Index) RemoveFile(path string) error {
	relPath, err := in.RelIndexPath(path)
	if err != nil {
		return err
	}
	delete(in.Files, relPath)
	return nil
}

func (in *Index) updateFileHashGiven(path, format, hash string, markAsMetaFile bool) error {
	// Remove format if equal to index hash format
	if in.HashFormat == format {
		format = ""
	}

	// Find in index
	relPath, err := in.RelIndexPath(path)
	if err != nil {
		return err
	}
	in.Files.updateFileEntry(relPath, format, hash, markAsMetaFile)
	return nil
}

// updateFile calculates the hash for a given path and updates it in the index
func (in *Index) updateFile(path string) error {
	var hashString string
	if viper.GetBool("no-internal-hashes") {
		hashString = ""
	} else {
		f, err := os.Open(path)
		if err != nil {
			return err
		}

		// Hash usage strategy (may change):
		// Just use SHA256, overwrite existing hash regardless of what it is
		// May update later to continue using the same hash that was already being used
		h, err := GetHashImpl("sha256")
		if err != nil {
			_ = f.Close()
			return err
		}
		if _, err := io.Copy(h, f); err != nil {
			_ = f.Close()
			return err
		}
		err = f.Close()
		if err != nil {
			return err
		}
		hashString = h.HashToString(h.Sum(nil))
	}

	// If the file has an extension of pw.toml, mark it as a metafile
	return in.updateFileHashGiven(path, "sha256", hashString, hasMetaExtension(path))
}

// ResolveIndexPath turns a path from the index into a file path on disk
func (in Index) ResolveIndexPath(p string) string {
	return filepath.Join(in.packRoot, filepath.FromSlash(p))
}

// RelIndexPath turns a file path on disk into a path from the index
func (in Index) RelIndexPath(p string) (string, error) {
	rel, err := filepath.Rel(in.packRoot, p)
	if err != nil {
		return "", err
	}
	return filepath.ToSlash(rel), nil
}

// ModListFile is the markdown file that `packwiz list --save` writes, unless told where to put it. It is made from
// the pack, so it isn't distributed with it (see ignoreDefaults).
const ModListFile = "MODS.md"

// DocFiles are the files in the pack's folder that describe the pack: its README, its licence and its release history.
// The index leaves them out (see ignoreDefaults), as they aren't something a launcher should install or something that
// changes the pack's version, but "packwiz modrinth export" puts whichever of them the pack has in the overrides of what
// it exports, so they travel with it without being asked for.
var DocFiles = []string{"README.md", "LICENSE", "CHANGELOG.md"}

// ServerConfigDir is the folder in the pack's folder of the files that only the server pack has, such as
// server.properties: "packwiz modrinth export --server" puts what is in it at the top of the server pack, where it replaces
// any file of the same path that the pack would otherwise put there. The index leaves it out (see ignoreDefaults), so
// nothing installs it on a client.
const ServerConfigDir = "serverconfig"

// ServerModListFile is the markdown file that "packwiz list --save --side server" writes, unless told where to put it:
// the list of the server's mods, which the server pack has in place of ModListFile, as it is in ServerConfigDir. It is
// a path relative to the pack's folder, with forward slashes.
const ServerModListFile = ServerConfigDir + "/" + ModListFile

// ServerPackSuffix ends the name of the zip that "packwiz modrinth export --server" writes unless told another, after the
// pack's name. The index leaves such files out (see ignoreDefaults), as it does .mrpack files.
const ServerPackSuffix = "-server.zip"

// IgnoreFile is the file in the pack's folder that lists what the index leaves out, in addition to ignoreDefaults. It is
// itself left out of the index.
const IgnoreFile = ".packwizignore"

var ignoreDefaults = []string{
	// Defaults (can be overridden with a negating pattern preceded with !)

	// Exclude Git metadata
	".git/**",
	".gitattributes",
	".gitignore",

	// Exclude macOS metadata
	".DS_Store",

	// Exclude exported Modrinth packs, and server packs
	"*.mrpack",
	"*" + ServerPackSuffix,

	// Exclude the files that are only for the server pack (see ServerConfigDir)
	ServerConfigDir + "/",

	// Exclude packwiz binaries, if the user puts them in their pack folder
	"packwiz.exe",
	"packwiz", // Note: also excludes packwiz/ as a directory - you can negate this pattern if you want a directory called packwiz

	// Exclude the files that describe the pack (see DocFiles): they are exported with it, but not through the index
	"README.md",
	"LICENSE",
	// Exclude the pack's own release history (see the changelog package)
	"CHANGELOG.md",
	"changelog.toml",
	// Exclude the file that sorts the pack's other files into categories (see FileCategoriesFile)
	FileCategoriesFile,
	// Exclude the list of the pack's mods that packwiz list --save writes
	ModListFile,
	".direnv/**",
	".editorconfig",
	".envrc",
	"flake.lock",
	"flake.nix",
	"lefthook.yml",
}

func readGitignore(path string) (*gitignore.GitIgnore, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		// TODO: check for read errors (and present them)
		return gitignore.CompileIgnoreLines(ignoreDefaults...), false
	}

	s := strings.Split(string(data), "\n")
	var lines []string
	lines = append(lines, ignoreDefaults...)
	lines = append(lines, s...)
	return gitignore.CompileIgnoreLines(lines...), true
}

// Refresh updates the hashes of all the files in the index, and adds new files to the index, showing its progress and
// anything it has to say on stdout
func (in *Index) Refresh() error {
	return in.refresh(mpb.New(), func(notice string) { fmt.Println(notice) })
}

// RefreshQuietly is Refresh without the progress bar, and with the notices it would print returned instead, as plain
// text: for a caller that has the screen to itself (such as the TUI) and can't have anything written over it.
func (in *Index) RefreshQuietly() (notices []string, err error) {
	err = in.refresh(mpb.New(mpb.WithOutput(io.Discard)), func(notice string) {
		notices = append(notices, ui.Strip(notice))
	})
	return notices, err
}

// refresh is Refresh, showing its progress in progressContainer and giving notify what it has to say
func (in *Index) refresh(progressContainer *mpb.Progress, notify func(notice string)) error {
	// TODO: If needed, multithreaded hashing
	// for i := 0; i < runtime.NumCPU(); i++ {}

	// Is case-sensitivity a problem?
	pathPF, _ := filepath.Abs(viper.GetString("pack-file"))
	pathIndex, _ := filepath.Abs(in.indexFile)

	pathIgnore, _ := filepath.Abs(filepath.Join(in.packRoot, IgnoreFile))
	ignore, ignoreExists := readGitignore(pathIgnore)

	var fileList []string
	err := filepath.WalkDir(in.packRoot, func(path string, info os.DirEntry, err error) error {
		if err != nil {
			// TODO: Handle errors on individual files properly
			return err
		}

		// Never ignore pack root itself (gitignore doesn't allow ignoring the root)
		if path == in.packRoot {
			return nil
		}

		if info.IsDir() {
			// Don't traverse ignored directories (consistent with Git handling of ignored dirs)
			if ignore.MatchesPath(path) {
				return fs.SkipDir
			}
			// Don't add directories to the file list
			return nil
		}

		// WalkDir doesn't follow symlinks, so a symlink pointing at a directory is
		// reported as a non-directory entry here; resolve it so it isn't opened as a
		// regular file (which fails with "is a directory") or added to the index.
		if info.Type()&fs.ModeSymlink != 0 {
			resolved, statErr := os.Stat(path)
			if statErr != nil || resolved.IsDir() {
				return nil
			}
		}
		// Exit if the files are the same as the pack/index files
		absPath, _ := filepath.Abs(path)
		if absPath == pathPF || absPath == pathIndex {
			return nil
		}
		if ignoreExists {
			if absPath == pathIgnore {
				return nil
			}
		}
		if ignore.MatchesPath(path) {
			return nil
		}

		fileList = append(fileList, path)
		return nil
	})
	if err != nil {
		return err
	}

	// A pack with a mod that reads the pack's files from a folder of its own (see ConfigDirResolver) tracks only that
	// folder and its metadata files
	fileList, err = in.keepConfigDirFiles(fileList, notify)
	if err != nil {
		return err
	}

	progress := progressContainer.AddBar(int64(len(fileList)),
		mpb.PrependDecorators(
			// simple name decorator
			decor.Name("Refreshing index..."),
			// decor.DSyncWidth bit enables column width synchronization
			decor.Percentage(decor.WCSyncSpace),
		),
		mpb.AppendDecorators(
			// replace ETA decorator with "done" message, OnComplete event
			decor.OnComplete(
				// ETA decorator with ewma age of 60
				decor.EwmaETA(decor.ET_STYLE_GO, 60), "done",
			),
		),
	)

	for _, v := range fileList {
		start := time.Now()

		err := in.updateFile(v)
		if err != nil {
			return err
		}

		progress.Increment(time.Since(start))
	}
	// Close bar
	progress.SetTotal(int64(len(fileList)), true) // If len = 0, we have to manually set complete to true
	progressContainer.Wait()

	// Check all the files exist, remove them if they don't
	for p, file := range in.Files {
		if !file.markedFound() {
			delete(in.Files, p)
		}
	}

	return nil
}

// Write saves the index file
func (in Index) Write() error {
	// Convert to indexTomlRepresentation
	rep := indexTomlRepresentation{
		HashFormat: in.HashFormat,
		Files:      in.Files.toTomlRep(),
	}

	// TODO: calculate and provide hash while writing?
	f, err := os.Create(in.indexFile)
	if err != nil {
		return err
	}

	enc := toml.NewEncoder(f)
	// Disable indentation
	enc.Indent = ""
	err = enc.Encode(rep)
	if err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}

// RefreshFileWithHash updates a file in the index, given a file hash and whether it should be marked as metafile or not
func (in *Index) RefreshFileWithHash(path, format, hash string, markAsMetaFile bool) error {
	if viper.GetBool("no-internal-hashes") {
		hash = ""
	}
	return in.updateFileHashGiven(path, format, hash, markAsMetaFile)
}

// SaveMod writes a mod's metadata file and records the file's new hash in the index. The index still has to be written
// afterwards, and the pack's record of its hash brought up to date (see Pack.SaveIndex).
func (in *Index) SaveMod(m *Mod) error {
	format, hash, err := m.Write()
	if err != nil {
		return err
	}
	return in.RefreshFileWithHash(m.GetFilePath(), format, hash, true)
}

// trimMetaExtension removes a mod metadata file's extension (MetaExtension, or the legacy MetaExtensionOld) from a
// file name, leaving it unchanged if it has neither.
func trimMetaExtension(fileName string) string {
	return strings.TrimSuffix(strings.TrimSuffix(fileName, MetaExtension), MetaExtensionOld)
}

// FindMod finds a mod in the index and returns its path and whether it has been found. modName can be the mod's
// slug (the name of its metadata file, without its extension, as this defaults to), that file name with its
// extension, or a path to the metadata file itself - absolute, or relative to the working directory.
func (in Index) FindMod(modName string) (string, bool) {
	modNameTrimmed := trimMetaExtension(modName)
	wantAbs, absErr := filepath.Abs(modName)
	for p, v := range in.Files {
		if v.IsMetaFile() {
			_, fileName := path.Split(p)
			fileTrimmed := trimMetaExtension(fileName)
			if fileTrimmed == modName || fileTrimmed == modNameTrimmed {
				return in.ResolveIndexPath(p), true
			}
			if absErr == nil {
				if candAbs, err := filepath.Abs(in.ResolveIndexPath(p)); err == nil && candAbs == wantAbs {
					return in.ResolveIndexPath(p), true
				}
			}
		}
	}
	return "", false
}

// ModMatch is a mod that some text fuzzily matches (see Index.SearchMods).
type ModMatch struct {
	// Path is where the mod's metadata file is, as FindMod gives it
	Path string
	// Name is the mod's name, and Slug the name of its metadata file without its extension
	Name, Slug string
	// Score is how well the text matched, which is higher the better
	Score int
}

// SearchMods finds the mods that text matches fuzzily (see package fuzzy) in their name or else their slug, the best
// match first: characters only have to be in order, several words all have to be found, and case doesn't matter. Of
// matches that are as good the one with the shorter name is first, as fzf has it, and then the one whose file comes first,
// so that the order is always the same. It is for when FindMod finds nothing: nothing is found by nothing.
func (in Index) SearchMods(text string) ([]ModMatch, error) {
	query := fuzzy.Parse(text)
	if query.Empty() {
		return nil, nil
	}
	var matches []ModMatch
	for _, p := range in.getAllMods() {
		mod, err := LoadMod(p)
		if err != nil {
			return nil, fmt.Errorf("failed to read metadata file %s: %w", p, err)
		}
		slug := trimMetaExtension(filepath.Base(p))
		if m, ok := query.Match(mod.Name, slug); ok {
			matches = append(matches, ModMatch{Path: p, Name: mod.Name, Slug: slug, Score: m.Score})
		}
	}
	slices.SortFunc(matches, func(a, b ModMatch) int {
		if a.Score != b.Score {
			return b.Score - a.Score
		}
		if an, bn := len([]rune(a.Name)), len([]rune(b.Name)); an != bn {
			return an - bn
		}
		return strings.Compare(a.Path, b.Path)
	})
	return matches, nil
}

// getAllMods finds paths to every metadata file (Mod) in the index
func (in Index) getAllMods() []string {
	var list []string
	for p, v := range in.Files {
		if v.IsMetaFile() {
			list = append(list, in.ResolveIndexPath(p))
		}
	}
	return list
}

// LoadAllMods reads all metadata files into Mod structs
func (in Index) LoadAllMods() ([]*Mod, error) {
	modPaths := in.getAllMods()
	mods := make([]*Mod, len(modPaths))
	for i, v := range modPaths {
		modData, err := LoadMod(v)
		if err != nil {
			return nil, fmt.Errorf("failed to read metadata file %s: %w", v, err)
		}
		mods[i] = &modData
	}
	return mods, nil
}
