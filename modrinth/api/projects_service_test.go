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
