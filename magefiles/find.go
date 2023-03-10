//go:build mage

package main

import (
	"os"
	"path/filepath"
	"regexp"
)

var findExclusions = []*regexp.Regexp{
	regexp.MustCompile(`^\.git`),
	regexp.MustCompile(`^\build`),
	regexp.MustCompile(`^\vendor`),
	regexp.MustCompile(`^\site/out`),
}

func find(match *regexp.Regexp) ([]string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return nil, err
	}

	var files []string
	return files, filepath.Walk(wd, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		for _, exclusion := range findExclusions {
			if exclusion.MatchString(path) {
				return filepath.SkipDir
			}
		}
		path, err = filepath.Rel(wd, path)
		if err != nil {
			return err
		}
		if match.MatchString(path) {
			files = append(files, path)
		}
		return nil
	})
}
