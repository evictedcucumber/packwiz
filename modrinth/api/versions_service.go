package api

import (
	"encoding/json"
	"net/http"
)

// VersionsService handles communication with the versions routes of the
// Modrinth API.
type VersionsService struct {
	client *Client
}

// ListVersionsOptions filters the results of ListVersions.
type ListVersionsOptions struct {
	Loaders      []string
	GameVersions []string
}

// ListVersions fetches all versions of a project, optionally filtered by
// loader and game version.
//
// Modrinth API docs: https://docs.modrinth.com/api/operations/getprojectversions/
func (s *VersionsService) ListVersions(projectIDOrSlug string, options ListVersionsOptions) ([]*Version, error) {
	req, err := s.client.newRequest(http.MethodGet, "project/"+projectIDOrSlug+"/version", nil)
	if err != nil {
		return nil, err
	}

	query := req.URL.Query()
	if len(options.Loaders) > 0 {
		out, err := json.Marshal(options.Loaders)
		if err != nil {
			return nil, err
		}
		query.Add("loaders", string(out))
	}
	if len(options.GameVersions) > 0 {
		out, err := json.Marshal(options.GameVersions)
		if err != nil {
			return nil, err
		}
		query.Add("game_versions", string(out))
	}
	req.URL.RawQuery = query.Encode()

	var versions []*Version
	if err := s.client.do(req, &versions); err != nil {
		return nil, err
	}
	return versions, nil
}

// Get fetches a single version by its ID.
//
// Modrinth API docs: https://docs.modrinth.com/api/operations/getversion/
func (s *VersionsService) Get(id string) (*Version, error) {
	req, err := s.client.newRequest(http.MethodGet, "version/"+id, nil)
	if err != nil {
		return nil, err
	}

	var version Version
	if err := s.client.do(req, &version); err != nil {
		return nil, err
	}
	return &version, nil
}

// GetMultiple fetches multiple versions by ID.
//
// Modrinth API docs: https://docs.modrinth.com/api/operations/getversions/
func (s *VersionsService) GetMultiple(ids []string) ([]*Version, error) {
	req, err := s.client.newRequest(http.MethodGet, "versions", nil)
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

	var versions []*Version
	if err := s.client.do(req, &versions); err != nil {
		return nil, err
	}
	return versions, nil
}
