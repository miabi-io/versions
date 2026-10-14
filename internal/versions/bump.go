package versions

import (
	"fmt"
	"slices"
	"strings"
	"time"
)

// BumpInput is one release to publish.
type BumpInput struct {
	Component    string
	Version      string
	Reason       string
	ChangelogURL string
	ReleasedAt   time.Time
	// Components is the tested bundle, "name=version" pairs; only a bundling component accepts it.
	Components map[string]string
	// SkipOlder turns "older than the published version" into a no-op instead of an error, for a release
	// that also carries another component (an older Miabi line pinning an older gateway).
	SkipOlder bool
}

// Bump records a release on its channel and regenerates the index. Re-publishing the current version is
// allowed (a re-run release workflow); publishing an older one is refused.
func Bump(root string, in BumpInput) (*Manifest, error) {
	comp, ok := Lookup(in.Component)
	if !ok {
		return nil, fmt.Errorf("unknown component %q", in.Component)
	}
	version := Normalize(in.Version)
	if !Valid(version) {
		return nil, fmt.Errorf("version %q is not x.y.z semver", in.Version)
	}
	reason := in.Reason
	if reason == "" {
		reason = "bugfix"
	}
	if !slices.Contains(Reasons, reason) {
		return nil, fmt.Errorf("reason %q must be one of %s", reason, strings.Join(Reasons, ", "))
	}
	if len(in.Components) > 0 && !comp.Bundles {
		return nil, fmt.Errorf("%s does not record a component bundle", comp.Name)
	}
	released := in.ReleasedAt
	if released.IsZero() {
		released = time.Now()
	}

	rel := &Release{
		Version:      version,
		ReleasedAt:   released.UTC().Truncate(time.Second),
		ReleaseURL:   fmt.Sprintf("https://github.com/%s/releases/tag/v%s", comp.Repo, version),
		ChangelogURL: in.ChangelogURL,
		Reason:       reason,
		Components:   in.Components,
	}
	if comp.Image != "" {
		rel.Image = comp.Image + ":" + version
	}

	m, err := ReadManifest(root, comp.Name)
	if err != nil {
		return nil, err
	}
	if in.SkipOlder && isOlder(m, version) {
		return m, nil
	}
	if IsPrerelease(version) {
		if m.Stable != nil && Compare(version, m.Stable.Version) <= 0 {
			return nil, fmt.Errorf("prerelease %s is not newer than stable %s", version, m.Stable.Version)
		}
		if m.Prerelease != nil && Compare(version, m.Prerelease.Version) < 0 {
			return nil, fmt.Errorf("prerelease %s is older than the published %s", version, m.Prerelease.Version)
		}
		m.Prerelease = rel
	} else {
		if m.Stable != nil && Compare(version, m.Stable.Version) < 0 {
			return nil, fmt.Errorf("stable %s is older than the published %s", version, m.Stable.Version)
		}
		m.Stable = rel
		if m.Prerelease != nil && Compare(m.Prerelease.Version, version) <= 0 {
			m.Prerelease = nil
		}
	}

	if err := WriteManifest(root, m); err != nil {
		return nil, err
	}
	return m, WriteIndex(root)
}

func isOlder(m *Manifest, version string) bool {
	if IsPrerelease(version) {
		return (m.Stable != nil && Compare(version, m.Stable.Version) <= 0) ||
			(m.Prerelease != nil && Compare(version, m.Prerelease.Version) < 0)
	}
	return m.Stable != nil && Compare(version, m.Stable.Version) < 0
}

// ParseComponents parses "gateway=1.1.1,agent=v0.6.2" into a bundle, normalising versions.
func ParseComponents(s string) (map[string]string, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, nil
	}
	out := map[string]string{}
	for _, pair := range strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == '\n' || r == ' ' }) {
		name, v, ok := strings.Cut(pair, "=")
		if !ok {
			return nil, fmt.Errorf("component %q is not name=version", pair)
		}
		name, v = strings.TrimSpace(name), Normalize(v)
		if _, known := Lookup(name); !known {
			return nil, fmt.Errorf("unknown component %q in bundle", name)
		}
		if !Valid(v) {
			return nil, fmt.Errorf("bundle version %s=%q is not x.y.z semver", name, v)
		}
		out[name] = v
	}
	return out, nil
}
