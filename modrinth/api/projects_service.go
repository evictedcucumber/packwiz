package api

import (
	"encoding/json"
	"net/http"
)

// ProjectsService handles communication with the projects routes of the
// Modrinth API.
type ProjectsService struct {
	client *Client
}

// Get fetches a single project by its ID or slug.
//
// Modrinth API docs: https://docs.modrinth.com/api/operations/getproject/
func (s *ProjectsService) Get(idOrSlug string) (*Project, error) {
	req, err := s.client.newRequest(http.MethodGet, "project/"+idOrSlug, nil)
	if err != nil {
		return nil, err
	}

	var project Project
	if err := s.client.do(req, &project); err != nil {
		return nil, err
	}
	return &project, nil
}

// GetMultiple fetches multiple projects by ID.
//
// Modrinth API docs: https://docs.modrinth.com/api/operations/getprojects/
func (s *ProjectsService) GetMultiple(ids []string) ([]*Project, error) {
	req, err := s.client.newRequest(http.MethodGet, "projects", nil)
	if err != nil {
		return nil, err
	}

	out, err := json.Marshal(ids)
	if err != nil {
		return nil, err
	}
	query := req.URL.Query()
	query.Add("ids", string(out))
	req.URL.RawQuery = query.Encode()

	var projects []*Project
	if err := s.client.do(req, &projects); err != nil {
		return nil, err
	}
	return projects, nil
}
