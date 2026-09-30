package api

import (
	"encoding/json"
	"net/http"
	"strconv"
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

// SearchOptions say what to search for.
type SearchOptions struct {
	// Query is the text to look for in the names and descriptions of projects
	Query string
	// Facets narrow what is found: each group is alternatives and a project has to match one of every group, so
	// {{"project_type:mod"}, {"versions:1.21.1"}} is mods for 1.21.1, and {{"categories:neoforge", "categories:fabric"}} is
	// for either
	Facets [][]string
	// Limit is how many projects to find at most, or 0 for what Modrinth gives by default
	Limit int
}

// Search finds projects by what they are called and what they say they are.
//
// Modrinth API docs: https://docs.modrinth.com/api/operations/searchprojects/
func (s *ProjectsService) Search(options SearchOptions) (*SearchResult, error) {
	req, err := s.client.newRequest(http.MethodGet, "search", nil)
	if err != nil {
		return nil, err
	}

	query := req.URL.Query()
	query.Set("query", options.Query)
	if len(options.Facets) > 0 {
		facets, err := json.Marshal(options.Facets)
		if err != nil {
			return nil, err
		}
		query.Set("facets", string(facets))
	}
	if options.Limit > 0 {
		query.Set("limit", strconv.Itoa(options.Limit))
	}
	req.URL.RawQuery = query.Encode()

	var result SearchResult
	if err := s.client.do(req, &result); err != nil {
		return nil, err
	}
	return &result, nil
}
