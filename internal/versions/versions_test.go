package versions

import (
	"os"
	"strings"
	"testing"
	"time"
)

var t0 = time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)

func mustBump(t *testing.T, root string, in BumpInput) *Manifest {
	t.Helper()
	if in.ReleasedAt.IsZero() {
		in.ReleasedAt = t0
	}
	m, err := Bump(root, in)
	if err != nil {
		t.Fatalf("bump %s %s: %v", in.Component, in.Version, err)
	}
	return m
}

func TestValid(t *testing.T) {
	for v, want := range map[string]bool{
		"1.10.18": true, "1.11.0-beta.2": true, "1.0.0+build.1": true,
		"v1.10.18": false, "1.10": false, "dev": false, "": false,
	} {
		if got := Valid(v); got != want {
			t.Errorf("Valid(%q) = %v, want %v", v, got, want)
		}
	}
}

func TestBumpChannels(t *testing.T) {
	root := t.TempDir()
	m := mustBump(t, root, BumpInput{Component: "miabi", Version: "v1.10.18"})
	if m.Stable.Image != "miabi/miabi:1.10.18" || m.Stable.ReleaseURL != "https://github.com/miabi-io/miabi/releases/tag/v1.10.18" {
		t.Fatalf("stable = %+v", m.Stable)
	}

	m = mustBump(t, root, BumpInput{Component: "miabi", Version: "v1.11.0-beta.1", Reason: "feature"})
	if m.Prerelease == nil || m.Prerelease.Version != "1.11.0-beta.1" || m.Stable.Version != "1.10.18" {
		t.Fatalf("after prerelease: %+v / %+v", m.Stable, m.Prerelease)
	}

	m = mustBump(t, root, BumpInput{Component: "miabi", Version: "1.10.19", Reason: "security"})
	if m.Prerelease == nil {
		t.Fatal("a patch below the prerelease must keep it")
	}

	m = mustBump(t, root, BumpInput{Component: "miabi", Version: "1.11.0"})
	if m.Prerelease != nil {
		t.Fatalf("stable caught up with the prerelease, it must be dropped: %+v", m.Prerelease)
	}
	if err := Validate(root, ""); err != nil {
		t.Fatal(err)
	}
}

func TestBumpRefuses(t *testing.T) {
	root := t.TempDir()
	mustBump(t, root, BumpInput{Component: "agent", Version: "0.6.2"})
	cases := map[string]BumpInput{
		"downgrade":            {Component: "agent", Version: "0.6.1"},
		"stale prerelease":     {Component: "agent", Version: "0.6.2-rc.1"},
		"unknown component":    {Component: "worker", Version: "1.0.0"},
		"bad version":          {Component: "agent", Version: "0.7"},
		"bad reason":           {Component: "agent", Version: "0.7.0", Reason: "urgent"},
		"bundle on non-bundle": {Component: "agent", Version: "0.7.0", Components: map[string]string{"runner": "0.2.0"}},
	}
	for name, in := range cases {
		in.ReleasedAt = t0
		if _, err := Bump(root, in); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
	if _, err := Bump(root, BumpInput{Component: "agent", Version: "0.6.2", ReleasedAt: t0}); err != nil {
		t.Errorf("re-publishing the current version must be allowed: %v", err)
	}
	m, err := Bump(root, BumpInput{Component: "agent", Version: "0.6.1", ReleasedAt: t0, SkipOlder: true})
	if err != nil || m.Stable.Version != "0.6.2" {
		t.Errorf("skip-older must leave 0.6.2 in place, got %+v, %v", m, err)
	}
}

func TestParseComponents(t *testing.T) {
	got, err := ParseComponents("gateway=v1.1.1, agent=0.6.2")
	if err != nil || got["gateway"] != "1.1.1" || got["agent"] != "0.6.2" {
		t.Fatalf("got %v, %v", got, err)
	}
	for _, bad := range []string{"gateway", "nope=1.0.0", "agent=latest"} {
		if _, err := ParseComponents(bad); err == nil {
			t.Errorf("ParseComponents(%q) accepted", bad)
		}
	}
}

func TestIndexIsDeterministic(t *testing.T) {
	root := t.TempDir()
	mustBump(t, root, BumpInput{Component: "runner", Version: "0.2.0"})
	mustBump(t, root, BumpInput{Component: "cli", Version: "0.20.0", ReleasedAt: t0.Add(time.Hour)})
	first, _ := os.ReadFile(IndexPath(root))
	if err := WriteIndex(root); err != nil {
		t.Fatal(err)
	}
	second, _ := os.ReadFile(IndexPath(root))
	if string(first) != string(second) {
		t.Fatal("regenerating an unchanged tree changed the index")
	}
	idx, _ := BuildIndex(root)
	if !idx.GeneratedAt.Equal(t0.Add(time.Hour)) || len(idx.Components) != 2 {
		t.Fatalf("index = %+v", idx)
	}
}

func TestValidateCatches(t *testing.T) {
	root := t.TempDir()
	mustBump(t, root, BumpInput{Component: "agent", Version: "0.6.2"})

	if err := os.WriteFile(ManifestPath(root, "agent"), []byte(strings.Replace(mustRead(t, ManifestPath(root, "agent")), `"0.6.2"`, `"0.6.3"`, 1)), 0o644); err != nil {
		t.Fatal(err)
	}
	err := Validate(root, "")
	if err == nil || !strings.Contains(err.Error(), "image") || !strings.Contains(err.Error(), "stale") {
		t.Fatalf("expected image and stale-index errors, got %v", err)
	}

	prev := t.TempDir()
	mustBump(t, prev, BumpInput{Component: "agent", Version: "0.7.0"})
	clean := t.TempDir()
	mustBump(t, clean, BumpInput{Component: "agent", Version: "0.6.2"})
	if err := Validate(clean, prev); err == nil || !strings.Contains(err.Error(), "backwards") {
		t.Fatalf("expected a downgrade error, got %v", err)
	}

	if err := os.WriteFile(ManifestPath(clean, "agent"), []byte(`{"$schema":"x","component":"agent","extra":1}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Validate(clean, ""); err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("expected an unknown-field error, got %v", err)
	}
}

func mustRead(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
