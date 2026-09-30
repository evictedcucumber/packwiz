package api

import (
	"encoding/json"
	"net/http"
	"testing"
)

func TestProjectsServiceGet(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("Method = %q, want GET", r.Method)
		}
		if r.URL.Path != "/project/sodium" {
			t.Errorf("Path = %q, want /project/sodium", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"AANobbMI","slug":"sodium","title":"Sodium","project_type":"mod"}`))
	})

	project, err := c.Projects.Get("sodium")
	if err != nil {
		t.Fatalf("Get() returned error: %v", err)
	}
	if project.ID == nil || *project.ID != "AANobbMI" {
		t.Errorf("ID = %v, want AANobbMI", project.ID)
	}
	if project.Slug == nil || *project.Slug != "sodium" {
		t.Errorf("Slug = %v, want sodium", project.Slug)
	}
	if project.Title == nil || *project.Title != "Sodium" {
		t.Errorf("Title = %v, want Sodium", project.Title)
	}
}

func TestProjectsServiceGetNotFound(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"not_found","description":"missing"}`))
	})

	_, err := c.Projects.Get("missing")
	if err == nil {
		t.Fatal("expected an error for a 404 response, got nil")
	}
	errResp, ok := err.(*ErrorResponse)
	if !ok {
		t.Fatalf("error type = %T, want *ErrorResponse", err)
	}
	if errResp.StatusCode != http.StatusNotFound {
		t.Errorf("StatusCode = %d, want 404", errResp.StatusCode)
	}
}

func TestProjectsServiceGetMultiple(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/projects" {
			t.Errorf("Path = %q, want /projects", r.URL.Path)
		}
		var ids []string
		if err := json.Unmarshal([]byte(r.URL.Query().Get("ids")), &ids); err != nil {
			t.Fatalf("failed to unmarshal ids query param: %v", err)
		}
		want := []string{"AANobbMI", "P7dR8mSH"}
		if len(ids) != len(want) || ids[0] != want[0] || ids[1] != want[1] {
			t.Errorf("ids = %v, want %v", ids, want)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`[{"id":"AANobbMI"},{"id":"P7dR8mSH"}]`))
	})

	projects, err := c.Projects.GetMultiple([]string{"AANobbMI", "P7dR8mSH"})
	if err != nil {
		t.Fatalf("GetMultiple() returned error: %v", err)
	}
	if len(projects) != 2 {
		t.Fatalf("got %d projects, want 2", len(projects))
	}
	if *projects[0].ID != "AANobbMI" || *projects[1].ID != "P7dR8mSH" {
		t.Errorf("unexpected project IDs: %v", []string{*projects[0].ID, *projects[1].ID})
	}
}

func TestProjectsServiceGetMultipleEmpty(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`[]`))
	})

	projects, err := c.Projects.GetMultiple(nil)
	if err != nil {
		t.Fatalf("GetMultiple() returned error: %v", err)
	}
	if len(projects) != 0 {
		t.Errorf("got %d projects, want 0", len(projects))
	}
}

func TestProjectsServiceSearch(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("Method = %q, want GET", r.Method)
		}
		if r.URL.Path != "/search" {
			t.Errorf("Path = %q, want /search", r.URL.Path)
		}
		query := r.URL.Query()
		if got := query.Get("query"); got != "sodium extra" {
			t.Errorf("query = %q, want what was searched for", got)
		}
		var facets [][]string
		if err := json.Unmarshal([]byte(query.Get("facets")), &facets); err != nil {
			t.Fatalf("failed to unmarshal the facets: %v", err)
		}
		if len(facets) != 2 || facets[0][0] != "project_type:mod" || facets[1][0] != "versions:1.21.1" || facets[1][1] != "versions:1.21" {
			t.Errorf("facets = %v, want the groups that were given, each with its alternatives", facets)
		}
		if got := query.Get("limit"); got != "5" {
			t.Errorf("limit = %q, want 5", got)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"hits":[{"project_id":"PtjYWJkn","slug":"sodium-extra","title":"Sodium Extra","description":"More options","author":"FlashyReese","project_type":"mod","client_side":"required","server_side":"unsupported","downloads":1234}],"total_hits":17}`))
	})

	result, err := c.Projects.Search(SearchOptions{
		Query:  "sodium extra",
		Facets: [][]string{{"project_type:mod"}, {"versions:1.21.1", "versions:1.21"}},
		Limit:  5,
	})
	if err != nil {
		t.Fatalf("Search() returned error: %v", err)
	}
	if result.TotalHits != 17 || len(result.Hits) != 1 {
		t.Fatalf("result = %+v, want one hit of 17", result)
	}
	hit := result.Hits[0]
	if *hit.ProjectID != "PtjYWJkn" || *hit.Slug != "sodium-extra" || *hit.Title != "Sodium Extra" ||
		*hit.Description != "More options" || *hit.Author != "FlashyReese" || *hit.ProjectType != "mod" ||
		*hit.ClientSide != "required" || *hit.ServerSide != "unsupported" || *hit.Downloads != 1234 {
		t.Errorf("hit = %+v, want what Modrinth said", hit)
	}
}

func TestProjectsServiceSearchWithNothingToNarrowItByAsksForNoFacets(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Has("facets") || r.URL.Query().Has("limit") {
			t.Errorf("query = %q, want no facets or limit", r.URL.RawQuery)
		}
		_, _ = w.Write([]byte(`{"hits":[],"total_hits":0}`))
	})
	result, err := c.Projects.Search(SearchOptions{Query: "x"})
	if err != nil || len(result.Hits) != 0 {
		t.Fatalf("Search() = %+v, %v, want no hits and no error", result, err)
	}
}

func TestProjectsServiceSearchFailure(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"invalid_input","description":"bad facets"}`))
	})
	if _, err := c.Projects.Search(SearchOptions{Query: "x"}); err == nil {
		t.Fatal("expected an error for a 400 response, got nil")
	}
}
