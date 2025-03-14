package main
import (
	"errors"
	"flag"
	"fmt"
	"log"
	"os/exec"
	"strings"
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
	var afterV2 bool
	var listMigs bool
	var migrationDirectory string
	var versionList string
	// If you only run with --patches, the upgrades that are minors are excluded.
	// Example being 1.0.0 -> 1.1.0 is a minor upgrade, so it's not included.
	flag.BoolVar(&includePatches, "patches", false, "Include patches releases")
	flag.BoolVar(&includeMinors, "minors", false, "Include minor releases")
	flag.BoolVar(&includeMajors, "majors", false, "Include major releases")
	flag.StringVar(&versionList, "versions", "", "Comma separated list of versions to use. This skips uses git tag to find tags.")
	flag.BoolVar(&afterV2, "after-v2", false, "Only include releases after v2.0.0")
	flag.BoolVar(&listMigs, "list", false, "List migrations")
	flag.StringVar(&migrationDirectory, "dir", "coderd/database/migrations", "Migration directory")
	flag.Parse()
	if !includePatches && !includeMinors && !includeMajors && versionList == "" {
		usage()
		return
	}
	var vList []string
	if versionList != "" {
		// Include all for printing purposes.
		includeMajors = true
		includeMinors = true
		includePatches = true
		vList = strings.Split(versionList, ",")
	}
	err := run(Options{
		VersionList:        vList,
		IncludePatches:     includePatches,
		IncludeMinors:      includeMinors,
		IncludeMajors:      includeMajors,
		AfterV2:            afterV2,
		ListMigrations:     listMigs,
		MigrationDirectory: migrationDirectory,
	})
	if err != nil {
		log.Fatal(err)
	}
}
func usage() {
	_, _ = fmt.Println("Usage: releasemigrations [--patches] [--minors] [--majors] [--list]")
	_, _ = fmt.Println("Choose at lease one of --patches, --minors, or --majors. You can choose all!")
	_, _ = fmt.Println("Must be run from the coder repo at the root.")
}
type Options struct {
	VersionList        []string
	IncludePatches     bool
	IncludeMinors      bool
	IncludeMajors      bool
	AfterV2            bool
	ListMigrations     bool
	MigrationDirectory string
}
func (o Options) Filter(tags []string) []string {
	if o.AfterV2 {
		for i, tag := range tags {
			if tag == "v2.0.0" {
				tags = tags[i:]
				break
			}
		}
	}
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
	var tags []string
	if len(opts.VersionList) > 0 {
		tags = opts.VersionList
	} else {
		var err error
		tags, err = gitTags()
		if err != nil {
			return fmt.Errorf("gitTags: %w", err)
		}
		tags = opts.Filter(tags)
	}
	patches := make([]string, 0)
	minors := make([]string, 0)
	majors := make([]string, 0)
	patchesHasMig := 0
	minorsHasMig := 0
	majorsHasMig := 0
	for i := 0; i < len(tags)-1; i++ {
		a := tags[i]
		b := tags[i+1]
		migrations, err := hasMigrationDiff(opts.MigrationDirectory, a, b)
		if err != nil {
			return fmt.Errorf("hasMigrationDiff %q->%q: %w", a, b, err)
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
			log.Printf("[%s] %d migrations added between %s and %s\n", vDiffType, len(migrations), a, b)
			if opts.ListMigrations {
				for _, migration := range migrations {
					log.Printf("\t%s", migration)
				}
			}
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
func hasMigrationDiff(dir string, a, b string) ([]string, error) {
	cmd := exec.Command("git", "diff",
		// Only added files
		"--diff-filter=A",
		"--name-only",
		a, b, dir)
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("%s\n%s", strings.Join(cmd.Args, " "), err)
	}
	if len(output) == 0 {
		return nil, nil
	}
	migrations := strings.Split(strings.TrimSpace(string(output)), "\n")
	filtered := make([]string, 0, len(migrations))
	for _, migration := range migrations {
		migration := migration
		if strings.Contains(migration, "fixtures") {
			continue
		}
		// Only show the ups
		if strings.HasSuffix(migration, ".down.sql") {
			continue
		}
		filtered = append(filtered, migration)
	}
	return filtered, nil
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
