package modrinth

type Pack struct {
	Game          string            `json:"game"`
	FormatVersion uint32            `json:"formatVersion"`
	VersionID     string            `json:"versionId"`
	Name          string            `json:"name"`
	Summary       string            `json:"summary,omitempty"`
	Files         []PackFile        `json:"files"`
	Dependencies  map[string]string `json:"dependencies"`
}

type PackFile struct {
	Path   string            `json:"path"`
	Hashes map[string]string `json:"hashes"`
	Env    *struct {
		Client string `json:"client"`
		Server string `json:"server"`
	} `json:"env"`
	Downloads []string `json:"downloads"`
	FileSize  uint32   `json:"fileSize"`
}
