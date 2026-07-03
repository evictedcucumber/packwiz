package modrinth

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

const (
	defaultBaseURL   = "https://api.modrinth.com/v2/"
	defaultUserAgent = "go-modrinth"
)

type Client struct {
	client    *http.Client
	BaseURL   *url.URL
	UserAgent string
	Token     string

	Projects *ProjectsService
	Versions *VersionsService
}

type service struct {
	client *Client
}

func NewClient(httpClient *http.Client) *Client {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	baseURL, _ := url.Parse(defaultBaseURL)

	c := &Client{
		client:    httpClient,
		BaseURL:   baseURL,
		UserAgent: defaultUserAgent,
	}
	c.Projects = &ProjectsService{client: c}
	c.Versions = &VersionsService{client: c}
	return c
}

func (c *Client) NewRequest(method string, urlStr string, body interface{}) (*http.Request, error) {
	u, err := c.BaseURL.Parse(urlStr)
	if err != nil {
		return nil, err
	}

	var buf io.ReadWriter
	if body != nil {
		buf = &bytes.Buffer{}
		enc := json.NewEncoder(buf)
		enc.SetEscapeHTML(false)
		if err := enc.Encode(body); err != nil {
			return nil, err
		}
	}

	req, err := http.NewRequest(method, u.String(), buf)
	if err != nil {
		return nil, err
	}

	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")
	if c.UserAgent != "" {
		req.Header.Set("User-Agent", c.UserAgent)
	}
	if c.Token != "" {
		req.Header.Set("Authorization", c.Token)
	}

	return req, nil
}

func (c *Client) Do(req *http.Request, v interface{}) (*http.Response, error) {
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	err = CheckResponse(resp)
	if err != nil {
		return resp, err
	}

	if v != nil {
		err = json.NewDecoder(resp.Body).Decode(v)
	}

	return resp, err
}

type ErrorResponse struct {
	Response *http.Response

	ErrorType   string `json:"error"`
	Description string `json:"description"`
}

func (r *ErrorResponse) Error() string {
	return fmt.Sprintf("%v %v: %d error type: '%v' - %v",
		r.Response.Request.Method, r.Response.Request.URL,
		r.Response.StatusCode, r.ErrorType, r.Description)
}

func (r *ErrorResponse) StatusCode() int {
	return r.Response.StatusCode
}

type NotFoundErrorResponse struct {
	Response *http.Response
}

func (r *NotFoundErrorResponse) Error() string {
	return fmt.Sprintf("%v %v: %d",
		r.Response.Request.Method, r.Response.Request.URL,
		r.Response.StatusCode)
}

func (r *NotFoundErrorResponse) StatusCode() int {
	return r.Response.StatusCode
}

func CheckResponse(r *http.Response) error {
	if r.StatusCode == http.StatusNotFound {
		return &NotFoundErrorResponse{Response: r}
	}
	if 200 <= r.StatusCode && r.StatusCode <= 299 {
		return nil
	}

	errorResponse := &ErrorResponse{Response: r}
	data, err := io.ReadAll(r.Body)
	if err == nil && data != nil {
		if err := json.Unmarshal(data, errorResponse); err != nil {
			return err
		}
	}

	return errorResponse
}

type ProjectsService service

type SearchOptions struct {
	Query   string
	Facets  [][]string
	Index   string
	Offset  int
	Limit   int
	Filters string
	Version string
}

type SearchResponse struct {
	Hits      []*SearchResult `json:"hits,omitempty"`
	Offset    *uint32         `json:"offset,omitempty"`
	Limit     *uint32         `json:"limit,omitempty"`
	TotalHits *uint32         `json:"total_hits,omitempty"`
}

type SearchResult struct {
	Slug          *string    `json:"slug,omitempty"`
	Title         *string    `json:"title,omitempty"`
	Description   *string    `json:"description,omitempty"`
	Categories    []string   `json:"categories,omitempty"`
	ClientSide    *string    `json:"client_side,omitempty"`
	ServerSide    *string    `json:"server_side,omitempty"`
	ProjectType   *string    `json:"project_type,omitempty"`
	Downloads     *uint32    `json:"downloads,omitempty"`
	IconURL       *string    `json:"icon_url,omitempty"`
	ProjectID     *string    `json:"project_id,omitempty"`
	Author        *string    `json:"author,omitempty"`
	Versions      []string   `json:"versions,omitempty"`
	Follows       *uint32    `json:"follows,omitempty"`
	DateCreated   *time.Time `json:"date_created,omitempty"`
	DateModified  *time.Time `json:"date_modified,omitempty"`
	LatestVersion *string    `json:"latest_version,omitempty"`
	Licence       *string    `json:"license,omitempty"`
	Gallery       []string   `json:"gallery,omitempty"`
}

func (s *ProjectsService) Search(options *SearchOptions) (*SearchResponse, error) {
	request, err := s.client.NewRequest(http.MethodGet, "search", nil)
	if err != nil {
		return nil, err
	}

	query := request.URL.Query()
	if options.Query != "" {
		query.Add("query", options.Query)
	}
	if options.Facets != nil && len(options.Facets) != 0 {
		out, err := json.Marshal(options.Facets)
		if err != nil {
			return nil, err
		}
		query.Add("facets", string(out))
	}
	if options.Index != "" {
		query.Add("index", options.Index)
	}
	if options.Offset != 0 {
		query.Add("offset", strconv.Itoa(options.Offset))
	}
	if options.Limit != 0 {
		query.Add("limit", strconv.Itoa(options.Limit))
	}
	if options.Filters != "" {
		query.Add("filters", options.Filters)
	}
	if options.Version != "" {
		query.Add("version", options.Version)
	}
	request.URL.RawQuery = query.Encode()

	var response SearchResponse
	if _, err = s.client.Do(request, &response); err != nil {
		return nil, err
	}

	return &response, nil
}

func (s *ProjectsService) Get(idOrSlug string) (*Project, error) {
	request, err := s.client.NewRequest(http.MethodGet, "project/"+idOrSlug, nil)
	if err != nil {
		return nil, err
	}

	var response Project
	if _, err = s.client.Do(request, &response); err != nil {
		return nil, err
	}

	return &response, nil
}

func (s *ProjectsService) GetMultiple(ids []string) ([]*Project, error) {
	request, err := s.client.NewRequest(http.MethodGet, "projects", nil)
	if err != nil {
		return nil, err
	}

	query := request.URL.Query()
	out, err := json.Marshal(ids)
	if err != nil {
		return nil, err
	}
	query.Add("ids", string(out))
	request.URL.RawQuery = query.Encode()

	var response []*Project
	if _, err = s.client.Do(request, &response); err != nil {
		return nil, err
	}

	return response, nil
}

type VersionsService service

type ListVersionsOptions struct {
	Loaders      []string
	GameVersions []string
	Featured     *bool
}

type File struct {
	Hashes   map[string]string `json:"hashes,omitempty"`
	URL      *string           `json:"url,omitempty"`
	Filename *string           `json:"filename,omitempty"`
	Primary  *bool             `json:"primary,omitempty"`
	Size     *uint32           `json:"size,omitempty"`
}

type Dependency struct {
	ProjectID      *string `json:"project_id,omitempty"`
	VersionID      *string `json:"version_id,omitempty"`
	DependencyType *string `json:"dependency_type,omitempty"`
}

type Version struct {
	Name          *string       `json:"name,omitempty"`
	VersionNumber *string       `json:"version_number,omitempty"`
	Changelog     *string       `json:"changelog,omitempty"`
	Dependencies  []*Dependency `json:"dependencies,omitempty"`
	GameVersions  []string      `json:"game_versions,omitempty"`
	VersionType   *string       `json:"version_type,omitempty"`
	Loaders       []string      `json:"loaders,omitempty"`
	Featured      *bool         `json:"featured,omitempty"`
	ID            *string       `json:"id,omitempty"`
	ProjectID     *string       `json:"project_id,omitempty"`
	AuthorID      *string       `json:"author_id,omitempty"`
	DatePublished *time.Time    `json:"date_published,omitempty"`
	Downloads     *uint32       `json:"downloads,omitempty"`
	ChangelogURL  *string       `json:"changelog_url,omitempty"`
	Files         []*File       `json:"files,omitempty"`
}

type Project struct {
	Slug                 *string         `json:"slug,omitempty"`
	Title                *string         `json:"title,omitempty"`
	Description          *string         `json:"description,omitempty"`
	Categories           []string        `json:"categories,omitempty"`
	AdditionalCategories []string        `json:"additional_categories,omitempty"`
	ClientSide           *string         `json:"client_side,omitempty"`
	ServerSide           *string         `json:"server_side,omitempty"`
	Body                 *string         `json:"body,omitempty"`
	IssuesURL            *string         `json:"issues_url,omitempty"`
	SourceURL            *string         `json:"source_url,omitempty"`
	WikiURL              *string         `json:"wiki_url,omitempty"`
	DiscordURL           *string         `json:"discord_url,omitempty"`
	DonationURLs         []interface{}   `json:"donation_urls,omitempty"`
	ProjectType          *string         `json:"project_type,omitempty"`
	Downloads            *uint32         `json:"downloads,omitempty"`
	IconURL              *string         `json:"icon_url,omitempty"`
	Colour               *uint32         `json:"color,omitempty"`
	ID                   *string         `json:"id,omitempty"`
	ProjectID            *string         `json:"project_id,omitempty"`
	Team                 *string         `json:"team,omitempty"`
	BodyURL              *string         `json:"body_url,omitempty"`
	ModeratorMessage     json.RawMessage `json:"moderator_message,omitempty"`
	Published            *time.Time      `json:"published,omitempty"`
	Updated              *time.Time      `json:"updated,omitempty"`
	Approved             *time.Time      `json:"approved,omitempty"`
	Followers            *uint32         `json:"followers,omitempty"`
	Status               *string         `json:"status,omitempty"`
	License              json.RawMessage `json:"license,omitempty"`
	Versions             []string        `json:"versions,omitempty"`
	GameVersions         []string        `json:"game_versions,omitempty"`
	Loaders              []string        `json:"loaders,omitempty"`
	Gallery              []*struct {
		Title       *string    `json:"title,omitempty"`
		Description *string    `json:"description,omitempty"`
		Featured    *bool      `json:"featured,omitempty"`
		Created     *time.Time `json:"created,omitempty"`
		URL         *string    `json:"url,omitempty"`
	} `json:"gallery,omitempty"`
}

func (s *VersionsService) ListVersions(projectIDOrSlug string, options ListVersionsOptions) ([]*Version, error) {
	request, err := s.client.NewRequest(http.MethodGet, "project/"+projectIDOrSlug+"/version", nil)
	if err != nil {
		return nil, err
	}

	query := request.URL.Query()
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
	if options.Featured != nil {
		query.Add("featured", strconv.FormatBool(*options.Featured))
	}
	request.URL.RawQuery = query.Encode()

	var response []*Version
	if _, err = s.client.Do(request, &response); err != nil {
		return nil, err
	}

	return response, nil
}

func (s *VersionsService) Get(idOrSlug string) (*Version, error) {
	request, err := s.client.NewRequest(http.MethodGet, "version/"+idOrSlug, nil)
	if err != nil {
		return nil, err
	}

	var response Version
	if _, err = s.client.Do(request, &response); err != nil {
		return nil, err
	}

	return &response, nil
}

func (s *VersionsService) GetMultiple(ids []string) ([]*Version, error) {
	request, err := s.client.NewRequest(http.MethodGet, "versions", nil)
	if err != nil {
		return nil, err
	}

	query := request.URL.Query()
	out, err := json.Marshal(ids)
	if err != nil {
		return nil, err
	}
	query.Add("ids", string(out))
	request.URL.RawQuery = query.Encode()

	var response []*Version
	if _, err = s.client.Do(request, &response); err != nil {
		return nil, err
	}

	return response, nil
}
