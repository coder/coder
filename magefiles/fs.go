//go:build mage

package main

import (
	"os"
	"path/filepath"
	"regexp"
	"time"

	"github.com/ammario/tlru"
	"github.com/coder/flog"
	"github.com/magefile/mage/mg"
)

var findExclusions = []*regexp.Regexp{
	regexp.MustCompile(`^\.git`),
	regexp.MustCompile(`^\build`),
	regexp.MustCompile(`^\vendor`),
	regexp.MustCompile(`^\site/out`),
	regexp.MustCompile(`^\site/node_modules`),
}

func cwd() string {
	wd, err := os.Getwd()
	if err != nil {
		mg.Fatalf(1, "failed to get working directory: %v", err)
	}
	return wd
}

// Cache regex compilation for ergonomics.
var regexCache = tlru.New[string](tlru.ConstantCost[*regexp.Regexp], 10000)

func fastRegex(s string) *regexp.Regexp {
	if r, _, ok := regexCache.Get(s); ok {
		return r
	}
	r := regexp.MustCompile(s)
	regexCache.Set(s, r, time.Hour)
	return r
}

func find(matchReg string) ([]string, error) {
	var files []string
	return files, filepath.Walk(cwd(), func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		for _, exclusion := range findExclusions {
			if exclusion.MatchString(path) {
				return filepath.SkipDir
			}
		}
		path, err = filepath.Rel(cwd(), path)
		if err != nil {
			return err
		}
		if fastRegex(matchReg).MatchString(path) {
			files = append(files, path)
		}
		return nil
	})
}

// destNewer returns true if the destination file is newer than any of the
// source files, describes as regex.
func destNewer(dest string, sourceRegexes ...string) bool {
	info, err := os.Stat(dest)
	if err != nil {
		if os.IsNotExist(err) {
			return false
		}
		mg.Fatalf(1, "failed to stat %q: %v", dest, err)
	}

	destModAt := info.ModTime()

	var (
		newer       = true
		filesWalked int
		offender    string
	)
	start := time.Now()
	err = filepath.Walk(cwd(), func(path string, info os.FileInfo, err error) error {
		filesWalked++
		if err != nil {
			return err
		}
		if path == "" {
			return nil
		}
		if info.ModTime().Before(destModAt) {
			return nil
		}

		for _, sourceRegex := range sourceRegexes {
			if !fastRegex(sourceRegex).MatchString(path) {
				continue
			}
			newer = true
			offender = path
			return filepath.SkipDir
		}
		return nil
	})
	end := time.Now()
	if err != nil {
		mg.Fatalf(1, "failed to walk: %v", err)
	}
	if mg.Verbose() {
		flog.Info("destNewer search took %v (walked %v files, result %v, offender %q)",
			end.Sub(start),
			filesWalked,
			newer,
			offender,
		)
	}
	return newer
}
