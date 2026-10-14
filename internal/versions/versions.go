// Package versions reads, writes and validates the static release manifests served from
// versions.miabi.io. Release workflows are the only writers, through Bump.
package versions

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"golang.org/x/mod/semver"
)

// APIVersion is the path prefix every manifest is served under. A breaking schema change ships as a new
// prefix next to this one, never in place: the URL is compiled into binaries that run for years.
const APIVersion = "v1"

// SchemaURL is the JSON Schema every manifest declares.
const SchemaURL = "https://versions.miabi.io/schema/v1.json"

// Reasons a release can be published for. "security" lets consumers show a stronger notice.
var Reasons = []string{"bugfix", "feature", "security", "other"}

// Component describes one published component.
type Component struct {
	Name string
	// Repo is the GitHub repository whose tags are the release versions.
	Repo string
	// Image is the container image without a tag; empty for components not shipped as an image.
	Image string
	// Bundles marks the component whose releases record the versions of the others they were tested with.
	Bundles bool
}

// Components is every component the service publishes, in index order.
var Components = []Component{
	{Name: "miabi", Repo: "miabi-io/miabi", Image: "miabi/miabi", Bundles: true},
	{Name: "agent", Repo: "miabi-io/agent", Image: "miabi/agent"},
	{Name: "runner", Repo: "miabi-io/runner", Image: "miabi/runner"},
	{Name: "gateway", Repo: "jkaninda/goma-gateway", Image: "jkaninda/goma-gateway"},
	{Name: "cli", Repo: "miabi-io/cli"},
}

// Lookup returns the named component.
func Lookup(name string) (Component, bool) {
	for _, c := range Components {
		if c.Name == name {
			return c, true
		}
	}
	return Component{}, false
}

// Release is one published version on a channel.
type Release struct {
	Version      string            `json:"version"`
	ReleasedAt   time.Time         `json:"released_at"`
	ReleaseURL   string            `json:"release_url"`
	ChangelogURL string            `json:"changelog_url,omitempty"`
	Reason       string            `json:"reason"`
	Image        string            `json:"image,omitempty"`
	Components   map[string]string `json:"components,omitempty"`
}

// Channels holds a component's current releases. Prerelease is only read by builds that are themselves
// prereleases, and is dropped once a stable release catches up with it.
type Channels struct {
	Stable     *Release `json:"stable,omitempty"`
	Prerelease *Release `json:"prerelease,omitempty"`
}

// Manifest is v1/<component>/latest.json.
type Manifest struct {
	Schema    string `json:"$schema"`
	Component string `json:"component"`
	Channels
}

// Index is v1/index.json: every component in one file, so a consumer needs a single request.
type Index struct {
	Schema string `json:"$schema"`
	// GeneratedAt is the newest release date in the index, not the wall clock, so regenerating an unchanged
	// tree is byte-identical and CI can detect a stale index.
	GeneratedAt time.Time           `json:"generated_at"`
	Components  map[string]Channels `json:"components"`
}

// Normalize strips the leading "v" a Git tag carries; manifests hold Docker-tag style versions.
func Normalize(v string) string { return strings.TrimPrefix(strings.TrimSpace(v), "v") }

// Valid reports whether v (without "v") is a full x.y.z semver version. semver.IsValid alone accepts
// shorthands like "1.2", which no release is tagged as.
func Valid(v string) bool {
	if v == "" || strings.HasPrefix(v, "v") {
		return false
	}
	core, _, _ := strings.Cut(v, "+")
	return semver.IsValid("v"+v) && semver.Canonical("v"+v) == "v"+core
}

// IsPrerelease reports whether v carries a prerelease suffix.
func IsPrerelease(v string) bool { return semver.Prerelease("v"+v) != "" }

// Compare orders two versions the way semver does.
func Compare(a, b string) int { return semver.Compare("v"+a, "v"+b) }

// ManifestPath is where a component's manifest lives under root.
func ManifestPath(root, component string) string {
	return filepath.Join(root, APIVersion, component, "latest.json")
}

// IndexPath is where the aggregate index lives under root.
func IndexPath(root string) string { return filepath.Join(root, APIVersion, "index.json") }

// ReadManifest loads a component manifest. A missing file returns an empty manifest, so the first release
// of a component needs no seeding.
func ReadManifest(root, component string) (*Manifest, error) {
	m := &Manifest{Schema: SchemaURL, Component: component}
	b, err := os.ReadFile(ManifestPath(root, component))
	if errors.Is(err, fs.ErrNotExist) {
		return m, nil
	}
	if err != nil {
		return nil, err
	}
	if err := decodeStrict(b, m); err != nil {
		return nil, fmt.Errorf("%s: %w", ManifestPath(root, component), err)
	}
	return m, nil
}

// WriteManifest writes a component manifest.
func WriteManifest(root string, m *Manifest) error {
	return writeJSON(ManifestPath(root, m.Component), m)
}

// BuildIndex assembles the index from every component manifest under root.
func BuildIndex(root string) (*Index, error) {
	idx := &Index{Schema: SchemaURL, Components: map[string]Channels{}}
	for _, c := range Components {
		m, err := ReadManifest(root, c.Name)
		if err != nil {
			return nil, err
		}
		if m.Stable == nil && m.Prerelease == nil {
			continue
		}
		idx.Components[c.Name] = m.Channels
		for _, r := range []*Release{m.Stable, m.Prerelease} {
			if r != nil && r.ReleasedAt.After(idx.GeneratedAt) {
				idx.GeneratedAt = r.ReleasedAt
			}
		}
	}
	return idx, nil
}

// WriteIndex regenerates v1/index.json from the component manifests.
func WriteIndex(root string) error {
	idx, err := BuildIndex(root)
	if err != nil {
		return err
	}
	return writeJSON(IndexPath(root), idx)
}

// Encode renders v exactly as files are written, so validation can compare bytes.
func Encode(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func writeJSON(path string, v any) error {
	b, err := Encode(v)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o644)
}

func decodeStrict(b []byte, v any) error {
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	return dec.Decode(v)
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
