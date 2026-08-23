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
