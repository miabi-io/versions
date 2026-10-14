package versions

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// Validate checks every manifest under root and that the index is current. With previous set (the tree
// before the change), it also refuses a channel moving backwards. It reports every problem, not the first.
func Validate(root, previous string) error {
	var errs []error
	add := func(format string, a ...any) { errs = append(errs, fmt.Errorf(format, a...)) }

	entries, err := os.ReadDir(filepath.Join(root, APIVersion))
	if err != nil {
		return err
	}
	for _, e := range entries {
		if e.IsDir() {
			if _, ok := Lookup(e.Name()); !ok {
				add("%s/%s: not a known component", APIVersion, e.Name())
			}
		}
	}

	for _, c := range Components {
		m, err := ReadManifest(root, c.Name)
		if err != nil {
			add("%v", err)
			continue
		}
		for _, p := range validateManifest(c, m) {
			add("%s: %s", c.Name, p)
		}
		if previous == "" {
			continue
		}
		old, err := ReadManifest(previous, c.Name)
		if err != nil {
			add("previous %v", err)
			continue
		}
		if old.Stable != nil && (m.Stable == nil || Compare(m.Stable.Version, old.Stable.Version) < 0) {
			add("%s: stable moved backwards from %s", c.Name, old.Stable.Version)
		}
	}

	idx, err := BuildIndex(root)
	if err != nil {
		add("index: %v", err)
	} else if want, err := Encode(idx); err != nil {
		add("index: %v", err)
	} else if got, err := os.ReadFile(IndexPath(root)); err != nil || !bytes.Equal(got, want) {
		add("%s/index.json is stale: run `go run ./cmd/versions index`", APIVersion)
	}
	return errors.Join(errs...)
}

func validateManifest(c Component, m *Manifest) []string {
	var problems []string
	if m.Schema != SchemaURL {
		problems = append(problems, fmt.Sprintf("$schema must be %s", SchemaURL))
	}
	if m.Component != c.Name {
		problems = append(problems, fmt.Sprintf("component is %q, want %q", m.Component, c.Name))
	}
	if m.Stable == nil && m.Prerelease == nil {
		return problems
	}
	if m.Stable != nil {
		if IsPrerelease(m.Stable.Version) {
			problems = append(problems, fmt.Sprintf("stable %s is a prerelease", m.Stable.Version))
		}
		problems = append(problems, validateRelease(c, "stable", m.Stable)...)
	}
	if m.Prerelease != nil {
		if !IsPrerelease(m.Prerelease.Version) {
			problems = append(problems, fmt.Sprintf("prerelease %s has no prerelease suffix", m.Prerelease.Version))
		}
		if m.Stable != nil && Compare(m.Prerelease.Version, m.Stable.Version) <= 0 {
			problems = append(problems, fmt.Sprintf("prerelease %s is not newer than stable %s", m.Prerelease.Version, m.Stable.Version))
		}
		problems = append(problems, validateRelease(c, "prerelease", m.Prerelease)...)
	}
	return problems
}

func validateRelease(c Component, channel string, r *Release) []string {
	var problems []string
	bad := func(format string, a ...any) { problems = append(problems, channel+": "+fmt.Sprintf(format, a...)) }
	if !Valid(r.Version) {
		bad("version %q is not x.y.z semver without a leading v", r.Version)
	}
	if r.ReleasedAt.IsZero() {
		bad("released_at is missing")
	}
	if !strings.HasPrefix(r.ReleaseURL, "https://") {
		bad("release_url must be https")
	}
	if r.ChangelogURL != "" && !strings.HasPrefix(r.ChangelogURL, "https://") {
		bad("changelog_url must be https")
	}
	if !slices.Contains(Reasons, r.Reason) {
		bad("reason %q must be one of %s", r.Reason, strings.Join(Reasons, ", "))
	}
	if want := c.Image + ":" + r.Version; c.Image != "" && r.Image != want {
		bad("image is %q, want %q", r.Image, want)
	}
	if c.Image == "" && r.Image != "" {
		bad("image set on a component that ships no image")
	}
	if len(r.Components) > 0 && !c.Bundles {
		bad("components set on a component that records no bundle")
	}
	for _, name := range sortedKeys(r.Components) {
		if _, ok := Lookup(name); !ok || name == c.Name {
			bad("components: %q is not another known component", name)
		}
		if !Valid(r.Components[name]) {
			bad("components: %s version %q is not x.y.z semver", name, r.Components[name])
		}
	}
	return problems
}
