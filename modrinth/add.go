package modrinth

import (
	"errors"
	"fmt"
	"regexp"
	"slices"

	"github.com/evictedcucumber/packwiz/core"
	"github.com/evictedcucumber/packwiz/internal/notice"
	modrinthApi "github.com/evictedcucumber/packwiz/modrinth/api"
)

// "packwiz modrinth add" prints what it finds and asks questions as it goes, which an interface that has the terminal to
// itself can't do. PlanAdd is the same work up to the point of changing the pack, with what it finds returned for the
// interface to show and ask about, and Apply is the change: neither prints anything or asks anything.

// AddOptions are what can be said about a project that is being added, besides which it is.
type AddOptions struct {
	// ReleaseType is the least stable kind of version to accept when looking for the latest (release, beta or alpha),
	// which for a project the pack has is what it was added with, and for one it hasn't the pack's default, if it is empty
	ReleaseType string
}

// AddExisting is what the pack has already of a project that is being added.
type AddExisting struct {
	// Name is what the pack calls it
	Name string
	// Current is the version it is at, or the name of its file for a mod that doesn't record its version
	Current string
	// UpToDate is whether it is at the version that would be added, so there is nothing to do
	UpToDate bool
	// Pinned is whether it can't be updated, as it is pinned
	Pinned bool
}

// AddDependency is a project that has to be added with the one that was asked for.
type AddDependency struct {
	Name, Version, File string
}

// AddPlan is what adding a project to the pack would do, which is done by Apply.
type AddPlan struct {
	// Project is what the project is called, and Slug what it is called in its address
	Project, Slug string
	// Version is the number of the version that would be added, and ReleaseType how stable it is
	Version, ReleaseType string
	// File is the file that would be installed, and Folder the folder of the pack it goes in
	File, Folder string
	// Side is where it would run
	Side string
	// Existing is what the pack already has of the project, or nil if it would be added
	Existing *AddExisting
	// Dependencies are the required projects that the pack doesn't have, at their latest versions, which can be added
	// along with it
	Dependencies []AddDependency
	// Notices are what looking it up said along the way, in plain text, such as that it is a Fabric mod that runs through
	// Sinytra Connector
	Notices []string

	plan *installPlan
}

// slugPattern is what a Modrinth slug or project ID can look like, as their addresses allow.
var slugPattern = regexp.MustCompile("^[a-zA-Z0-9!@$()`.+,_\"-]{3,64}$")

// PlanAdd works out what adding a project would do. ref is what says which: the address of its page on Modrinth, which
// can be for a version of it, the address of a file, or its slug or ID. It needs the network, and changes nothing.
func PlanAdd(pack core.Pack, index core.Index, ref string, options AddOptions) (*AddPlan, error) {
	if options.ReleaseType != "" && !core.IsValidReleaseType(options.ReleaseType) {
		return nil, fmt.Errorf("%q isn't a release type; it must be one of: release, beta, alpha", options.ReleaseType)
	}

	var projectID, versionNumber, versionID, filename string
	if err := parseUrl(ref, &projectID, &versionNumber, &versionID, &filename); err != nil {
		// Modrinth takes a slug or an ID where it takes a project, so that is as good as an address
		if !slugPattern.MatchString(ref) {
			return nil, fmt.Errorf("%q isn't a Modrinth address, slug or project ID", ref)
		}
		projectID, versionNumber, versionID, filename = ref, "", "", ""
	}

	var plan *AddPlan
	var err error
	notices := notice.Collect(func() {
		plan, err = planAdd(pack, &index, projectID, versionNumber, versionID, filename, options.ReleaseType)
	})
	if err != nil {
		return nil, err
	}
	for _, n := range notices {
		if n.Level != notice.Muted {
			plan.Notices = append(plan.Notices, n.Text)
		}
	}
	return plan, nil
}

func planAdd(pack core.Pack, index *core.Index, projectID, versionNumber, versionID, filename, releaseType string) (*AddPlan, error) {
	project, version, err := resolveTarget(pack, index, projectID, versionNumber, versionID, releaseType)
	if err != nil {
		return nil, err
	}
	plan, err := newInstallPlan(project, version, filename, pack, index, releaseType)
	if err != nil {
		return nil, err
	}
	// Found now, as adding a project of a kind that can't be added is better found before its dependencies are looked up
	folder, err := getProjectTypeFolder(*project.ProjectType, version.Loaders, pack.GetCompatibleLoaders())
	if err != nil {
		return nil, err
	}

	result := &AddPlan{
		Project: *project.Title, Version: versionNumberOf(version), ReleaseType: versionReleaseType(version),
		File: *plan.file.Filename, Folder: folder, Side: getSide(project), plan: plan,
	}
	if project.Slug != nil {
		result.Slug = *project.Slug
	}
	if result.Side == "" {
		result.Side = core.UniversalSide
	}
	if plan.existing != nil {
		result.Existing = &AddExisting{Name: plan.existing.Name, Current: plan.existing.DisplayVersion(), UpToDate: plan.upToDate, Pinned: plan.pinned}
		if plan.upToDate || plan.pinned {
			// Nothing is done to it, so what it would need isn't looked up
			return result, nil
		}
	}

	if err := plan.lookUpDependencies(index); err != nil {
		return nil, err
	}
	for _, d := range plan.deps {
		result.Dependencies = append(result.Dependencies, AddDependency{Name: *d.projectInfo.Title, Version: versionNumberOf(d.versionInfo), File: *d.fileInfo.Filename})
	}
	return result, nil
}

// AddResult is what adding a project did.
type AddResult struct {
	// Project is what the project is called, and File the file it installs
	Project, File string
	// Updated is whether the pack had the project, which was updated, rather than it being added
	Updated bool
	// Dependencies are the names of the dependencies that were added with it
	Dependencies []string
	// Notices are what was said along the way, such as that a mod was put on both sides as another needs it on the client
	Notices []string
}

// Apply adds the project, or updates it if the pack has it, and with withDependencies the dependencies that were found
// too. It works on the pack as it is now, so a project that has been added or pinned since it was planned is noticed.
func (p *AddPlan) Apply(withDependencies bool) (*AddResult, error) {
	if p.Existing != nil && (p.Existing.UpToDate || p.Existing.Pinned) {
		return nil, errors.New("there is nothing to do: " + p.Existing.Name + " is up to date or pinned")
	}
	pack, err := core.LoadPack()
	if err != nil {
		return nil, err
	}
	index, err := pack.LoadIndex()
	if err != nil {
		return nil, err
	}

	// What the pack has now, which may not be what it had when this was planned
	plan := *p.plan
	plan.pack = pack
	existing, err := findInstalledMod(&index, *plan.project.ID)
	if err != nil {
		return nil, err
	}
	switch {
	case (existing == nil) != (plan.existing == nil):
		return nil, errors.New("the pack has changed since this was planned: " + p.Project + " has been added or removed, so look it up again")
	case existing != nil && existing.Pin:
		return nil, fmt.Errorf("%q is pinned; unpin it to allow updating", existing.Name)
	}
	plan.existing = existing

	result := &AddResult{Project: p.Project, File: p.File, Updated: existing != nil}
	notices := notice.Collect(func() {
		err = plan.apply(&index, withDependencies, func(d depMetadataStore) {
			result.Dependencies = append(result.Dependencies, *d.projectInfo.Title)
		})
	})
	if err != nil {
		return nil, err
	}
	for _, n := range notices {
		if n.Level != notice.Muted {
			result.Notices = append(result.Notices, n.Text)
		}
	}
	return result, nil
}

// The kinds of project that can be searched for, which are what Modrinth calls them.
const (
	KindMod          = "mod"
	KindResourcePack = "resourcepack"
	KindShader       = "shader"
)

// Found is a project that a search found.
type Found struct {
	// ID and Slug tell which project it is; either can be given to PlanAdd
	ID, Slug string
	Title    string
	// Description is what the project says of itself, in a line
	Description string
	Author      string
	Kind        string
	Downloads   int
	// ClientSide and ServerSide are what Modrinth says of whether it is needed on each: "required", "optional",
	// "unsupported" or "unknown"
	ClientSide, ServerSide string
	// InPack is whether the pack has the project already
	InPack bool
}

// SearchResults is what a search found.
type SearchResults struct {
	Found []Found
	// Total is how many projects match, which can be more than were returned
	Total int
}

// searchLimit is how many projects a search returns, which is as many as fit on a screen.
const searchLimit = 25

// Search looks for projects of a kind that suit the pack by what they are called and say they are: for the game versions the
// pack accepts and, for mods, for its mod loader. It needs the network.
func Search(pack core.Pack, index core.Index, query, kind string) (*SearchResults, error) {
	facets := [][]string{{"project_type:" + kind}}

	versions, err := pack.GetSupportedMCVersions()
	if err != nil {
		return nil, err
	}
	group := make([]string, len(versions))
	for i, v := range versions {
		group[i] = "versions:" + v
	}
	facets = append(facets, group)

	var installedProjects []string
	notice.Collect(func() { installedProjects = getInstalledProjectIDs(&index) })
	if kind == KindMod {
		var loaders []string
		for _, loader := range pack.GetCompatibleLoaders() {
			loaders = append(loaders, "categories:"+loader)
		}
		// Sinytra Connector runs mods that are made for Fabric, which is why the pack can have them (see runsFabricMods)
		if runsFabricMods(pack, installedProjects) {
			loaders = append(loaders, "categories:fabric")
		}
		if len(loaders) > 0 {
			facets = append(facets, loaders)
		}
	}

	result, err := mrDefaultClient.Projects.Search(modrinthApi.SearchOptions{Query: query, Facets: facets, Limit: searchLimit})
	if err != nil {
		return nil, fmt.Errorf("the search failed: %w", err)
	}

	found := &SearchResults{Total: result.TotalHits}
	for _, hit := range result.Hits {
		if hit == nil || hit.ProjectID == nil || hit.Title == nil {
			continue
		}
		f := Found{ID: *hit.ProjectID, Title: *hit.Title, Kind: kind, InPack: slices.Contains(installedProjects, *hit.ProjectID)}
		if hit.Slug != nil {
			f.Slug = *hit.Slug
		}
		if hit.Description != nil {
			f.Description = *hit.Description
		}
		if hit.Author != nil {
			f.Author = *hit.Author
		}
		if hit.Downloads != nil {
			f.Downloads = *hit.Downloads
		}
		if hit.ClientSide != nil {
			f.ClientSide = *hit.ClientSide
		}
		if hit.ServerSide != nil {
			f.ServerSide = *hit.ServerSide
		}
		found.Found = append(found.Found, f)
	}
	return found, nil
}
