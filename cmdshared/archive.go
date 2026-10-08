package cmdshared

import (
	"archive/zip"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
)

// Archive is where exported files are put, each by its path in it with forward slashes: a zip, or a folder.
type Archive interface {
	Add(name string, content io.Reader) error
}

// ZipArchive puts files in a zip.
type ZipArchive struct {
	*zip.Writer
}

func (z ZipArchive) Add(name string, content io.Reader) error {
	file, err := z.Create(name)
	if err != nil {
		return err
	}
	_, err = io.Copy(file, content)
	return err
}

// AddMode is Add with the file's permissions given, such as 0o755 for a script that is to be run once it is unzipped.
func (z ZipArchive) AddMode(name string, content io.Reader, mode fs.FileMode) error {
	header := &zip.FileHeader{Name: name, Method: zip.Deflate}
	header.SetMode(mode)
	file, err := z.CreateHeader(header)
	if err != nil {
		return err
	}
	_, err = io.Copy(file, content)
	return err
}

// DirArchive puts files in a folder on disk, replacing any that are there. Each is written to a file of its own and then
// put in place, so a program that has the old one open (a server that has loaded a mod) keeps reading what it opened.
type DirArchive struct {
	Root string
	// Added are the paths of the files that were put in it
	Added map[string]bool
}

// NewDirArchive is a DirArchive that puts files in root.
func NewDirArchive(root string) *DirArchive {
	return &DirArchive{Root: root, Added: map[string]bool{}}
}

func (d *DirArchive) Add(name string, content io.Reader) error {
	rel := filepath.FromSlash(name)
	if !filepath.IsLocal(rel) {
		return fmt.Errorf("%s is outside the folder", name)
	}
	dest := filepath.Join(d.Root, rel)
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(dest), ".packwiz-*")
	if err != nil {
		return err
	}
	if _, err = io.Copy(tmp, content); err == nil {
		err = tmp.Close()
	} else {
		_ = tmp.Close()
	}
	if err == nil {
		err = os.Rename(tmp.Name(), dest)
	}
	if err != nil {
		_ = os.Remove(tmp.Name())
		return err
	}
	d.Added[name] = true
	return nil
}
