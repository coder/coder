package main

import (
	"flag"
	"fmt"
	"log"
	"os/exec"
	"strings"

	"golang.org/x/xerrors"

	"golang.org/x/mod/semver"
)

// main will print out the number of migrations added between each release.
// All upgrades are categorized as either major, minor, or patch based on semver.
//
// This isn't an exact science and is opinionated. Upgrade paths are not
// always strictly linear from release to release. Users can skip patches for
// example.
func main() {
	var includePatches bool
	var includeMinors bool
	var includeMajors bool
	// If you only run with --patches, the upgrades that are minors are excluded.
	// Example being 1.0.0 -> 1.1.0 is a minor upgrade, so it's not included.
	flag.BoolVar(&includePatches, "patches", false, "Include patches releases")
	flag.BoolVar(&includeMinors, "minors", false, "Include minor releases")
	flag.BoolVar(&includeMajors, "majors", false, "Include major releases")
	flag.Parse()

	if !includePatches && !includeMinors && !includeMajors {
		usage()
		return
	}

	err := run(Options{
		IncludePatches: includePatches,
		IncludeMinors:  includeMinors,
		IncludeMajors:  includeMajors,
	})
	if err != nil {
		log.Fatal(err)
	}
}

func usage() {
	fmt.Println("Usage: releasemigrations [--patches] [--minors] [--majors]")
	fmt.Println("Choose at lease one of --patches, --minors, or --majors. You can choose all!")
}

type Options struct {
	IncludePatches bool
	IncludeMinors  bool
	IncludeMajors  bool
}

func (o Options) Filter(tags []string) []string {
	if o.IncludeMajors && o.IncludeMinors && o.IncludePatches {
		return tags
	}

	filtered := make([]string, 0, len(tags))
	current := tags[0]
	filtered = append(filtered, current)
	for i := 1; i < len(tags); i++ {
		a := current
		current = tags[i]

		vDiffType := versionDiff(a, tags[i])
		if !o.IncludeMajors && vDiffType == "major" {
			continue
		}
		if !o.IncludeMinors && vDiffType == "minor" {
			// This isn't perfect, but we need to include
			// the first minor release for the first patch to work.
			// Eg: 1.0.0 -> 1.1.0 -> 1.1.1
			//	If we didn't include 1.1.0, then the 1.1.1 patch would
			// 	apply to 1.0.0
			if !o.IncludePatches {
				continue
			}
		}
		if !o.IncludePatches && vDiffType == "patch" {
			continue
		}
		filtered = append(filtered, tags[i])
	}

	return filtered
}

func run(opts Options) error {
	tags, err := gitTags()
	if err != nil {
		return xerrors.Errorf("gitTags: %w", err)
	}
	tags = opts.Filter(tags)

	patches := make([]string, 0)
	minors := make([]string, 0)
	majors := make([]string, 0)
	patchesHasMig := 0
	minorsHasMig := 0
	majorsHasMig := 0

	for i := 0; i < len(tags)-1; i++ {
		a := tags[i]
		b := tags[i+1]

		migrations, err := hasMigrationDiff(a, b)
		if err != nil {
			return xerrors.Errorf("hasMigrationDiff %q->%q: %w", a, b, err)
		}

		vDiff := fmt.Sprintf("%s->%s", a, b)
		vDiffType := versionDiff(a, b)
		skipPrint := true
		switch vDiffType {
		case "major":
			majors = append(majors, vDiff)
			if len(migrations) > 0 {
				majorsHasMig++
			}
			skipPrint = !opts.IncludeMajors
		case "minor":
			minors = append(minors, vDiff)
			if len(migrations) > 0 {
				minorsHasMig++
			}
			skipPrint = !opts.IncludeMinors
		case "patch":
			patches = append(patches, vDiff)
			if len(migrations) > 0 {
				patchesHasMig++
			}
			skipPrint = !opts.IncludePatches
		}

		if skipPrint {
			continue
		}

		if migrations != nil {
			log.Printf("[%s] %d migrations added between %s and %s\n", vDiffType, len(migrations)/2, a, b)
			//for _, migration := range migrations {
			//	log.Printf("  %s\n", migration)
			//}
		} else {
			log.Printf("[%s] No migrations added between %s and %s\n", vDiffType, a, b)
		}
	}

	log.Printf("Patches: %d (%d with migrations)\n", len(patches), patchesHasMig)
	log.Printf("Minors: %d (%d with migrations)\n", len(minors), minorsHasMig)
	log.Printf("Majors: %d (%d with migrations)\n", len(majors), majorsHasMig)

	return nil
}

func versionDiff(a, b string) string {
	ac, bc := semver.Canonical(a), semver.Canonical(b)
	if semver.Major(ac) != semver.Major(bc) {
		return "major"
	}
	if semver.MajorMinor(ac) != semver.MajorMinor(bc) {
		return "minor"
	}
	return "patch"
}

func hasMigrationDiff(a, b string) ([]string, error) {
	cmd := exec.Command("git", "diff",
		// Only added files
		"--diff-filter=A",
		"--name-only",
		a, b, "coderd/database/migrations")
	output, err := cmd.Output()
	if err != nil {
		return nil, xerrors.Errorf("%s\n%s", strings.Join(cmd.Args, " "), err)
		return nil, err
	}
	if len(output) == 0 {
		return nil, nil
	}

	migrations := strings.Split(string(output), "\n")
	return migrations, nil
}

func gitTags() ([]string, error) {
	cmd := exec.Command("git", "tag")
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	tags := strings.Split(string(output), "\n")

	// Sort by semver
	semver.Sort(tags)

	filtered := make([]string, 0, len(tags))
	for _, tag := range tags {
		if tag != "" && semver.IsValid(tag) {
			filtered = append(filtered, tag)
		}
	}

	return filtered, nil
}
