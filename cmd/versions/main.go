// Command versions maintains the manifests served from versions.miabi.io.
//
//	versions bump -component agent -version v0.6.3 [-reason security] [-changelog-url URL]
//	              [-released-at RFC3339] [-components gateway=1.1.1,agent=0.6.3] [-skip-older] [-dir public]
//	versions index    [-dir public]
//	versions validate [-dir public] [-previous base/public]
package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/miabi-io/versions/internal/versions"
)

func main() {
	if len(os.Args) < 2 {
		usage()
	}
	var err error
	switch os.Args[1] {
	case "bump":
		err = bump(os.Args[2:])
	case "index":
		fs := flag.NewFlagSet("index", flag.ExitOnError)
		dir := fs.String("dir", "public", "site root")
		_ = fs.Parse(os.Args[2:])
		err = versions.WriteIndex(*dir)
	case "validate":
		fs := flag.NewFlagSet("validate", flag.ExitOnError)
		dir := fs.String("dir", "public", "site root")
		previous := fs.String("previous", "", "site root before the change, to refuse downgrades")
		_ = fs.Parse(os.Args[2:])
		if err = versions.Validate(*dir, *previous); err == nil {
			fmt.Println("ok")
		}
	default:
		usage()
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "versions:", err)
		os.Exit(1)
	}
}

func bump(args []string) error {
	fs := flag.NewFlagSet("bump", flag.ExitOnError)
	dir := fs.String("dir", "public", "site root")
	component := fs.String("component", "", "component name")
	version := fs.String("version", "", "released version or tag, e.g. v1.10.19")
	reason := fs.String("reason", "bugfix", "bugfix | feature | security | other")
	changelog := fs.String("changelog-url", "", "release notes URL")
	releasedAt := fs.String("released-at", "", "release time, RFC3339 (default now)")
	bundle := fs.String("components", "", "tested bundle, name=version pairs (miabi only)")
	skipOlder := fs.Bool("skip-older", false, "treat a version not newer than the published one as a no-op")
	_ = fs.Parse(args)

	in := versions.BumpInput{Component: *component, Version: *version, Reason: *reason, ChangelogURL: *changelog, SkipOlder: *skipOlder}
	if *releasedAt != "" {
		t, err := time.Parse(time.RFC3339, *releasedAt)
		if err != nil {
			return fmt.Errorf("released-at: %w", err)
		}
		in.ReleasedAt = t
	}
	comps, err := versions.ParseComponents(*bundle)
	if err != nil {
		return err
	}
	in.Components = comps

	m, err := versions.Bump(*dir, in)
	if err != nil {
		return err
	}
	fmt.Printf("%s: stable=%s prerelease=%s\n", m.Component, versionOf(m.Stable), versionOf(m.Prerelease))
	return nil
}

func versionOf(r *versions.Release) string {
	if r == nil {
		return "-"
	}
	return r.Version
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: versions bump|index|validate [flags]")
	os.Exit(2)
}
