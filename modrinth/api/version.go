package api

import "time"

// Dependency is a dependency of a project version on another project or
// version.
type Dependency struct {
	ProjectID      *string `json:"project_id,omitempty"`
	VersionID      *string `json:"version_id,omitempty"`
	DependencyType *string `json:"dependency_type,omitempty"`
}

// File is a file attached to a project version.
type File struct {
	Hashes   map[string]string `json:"hashes,omitempty"`
	URL      *string           `json:"url,omitempty"`
	Filename *string           `json:"filename,omitempty"`
	Primary  *bool             `json:"primary,omitempty"`
}

// Version is a version of a Modrinth project.
//
// Modrinth API docs: https://docs.modrinth.com/api/operations/getversion/
type Version struct {
	ID            *string       `json:"id,omitempty"`
	ProjectID     *string       `json:"project_id,omitempty"`
	VersionNumber *string       `json:"version_number,omitempty"`
	GameVersions  []string      `json:"game_versions,omitempty"`
	Loaders       []string      `json:"loaders,omitempty"`
	DatePublished *time.Time    `json:"date_published,omitempty"`
	Dependencies  []*Dependency `json:"dependencies,omitempty"`
	Files         []*File       `json:"files,omitempty"`
}
