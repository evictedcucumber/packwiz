package api

// Project is a Modrinth project (mod, resource pack, shader, etc).
//
// Modrinth API docs: https://docs.modrinth.com/api/operations/getproject/
type Project struct {
	ID          *string  `json:"id,omitempty"`
	Slug        *string  `json:"slug,omitempty"`
	Title       *string  `json:"title,omitempty"`
	ProjectType *string  `json:"project_type,omitempty"`
	ClientSide  *string  `json:"client_side,omitempty"`
	ServerSide  *string  `json:"server_side,omitempty"`
	Versions    []string `json:"versions,omitempty"`
}

// SearchHit is a project that a search found: what is needed to tell it from the others and to add it.
type SearchHit struct {
	ProjectID   *string `json:"project_id,omitempty"`
	Slug        *string `json:"slug,omitempty"`
	Title       *string `json:"title,omitempty"`
	Description *string `json:"description,omitempty"`
	Author      *string `json:"author,omitempty"`
	ProjectType *string `json:"project_type,omitempty"`
	ClientSide  *string `json:"client_side,omitempty"`
	ServerSide  *string `json:"server_side,omitempty"`
	Downloads   *int    `json:"downloads,omitempty"`
}

// SearchResult is what a search found.
type SearchResult struct {
	Hits []*SearchHit `json:"hits"`
	// TotalHits is how many projects match, which can be more than the hits that were asked for
	TotalHits int `json:"total_hits"`
}
