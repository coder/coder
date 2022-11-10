package main

import (
	"bufio"
	"fmt"
	"os"
	"path"
	"strings"

	"golang.org/x/xerrors"
)

const (
	dir          = "queries"
	beginQuery   = "-- name:"
	targetPhrase = "@gen_copy"
)

func main() {
	err := run()
	if err != nil {
		panic(err)
	}
}

func run() error {
	queryFiles, err := os.ReadDir(dir)
	if err != nil {
		return err
	}

	// get all files in queries dir
	for _, queryFile := range queryFiles {
		if !strings.HasSuffix(queryFile.Name(), ".sql") && strings.HasSuffix(queryFile.Name(), "copies.sql") {
			continue
		}
		lines, err := readLines(path.Join(dir, queryFile.Name()))
		if err != nil {
			return err
		}

		// read lines of each queries file
		var copies []string
		for i, l := range lines {
			if strings.Contains(l, beginQuery) {
				// read query to end
				var q []string
				for j := i; j < len(lines); j++ {
					q = append(q, lines[j])
					if strings.Contains(lines[j], ";") {
						break
					}
				}

				type variant struct {
					name    string
					output  string
					variant string
					line    int
				}
				// read each line of query and look for target phrase
				var variants []variant
				for j, l := range q {
					if strings.Contains(l, targetPhrase) {
						// collect all phrases after the target phrase
						found := false
						fields := strings.Fields(l)
						for k := 0; k < len(fields); k++ {
							if !found && fields[k] == targetPhrase {
								found = true
								continue
							}
							if !found {
								continue
							}
							if len(fields) <= k+2 {
								return xerrors.Errorf("malformed line with target phrase, must have 3 values after target phrase: %s", l)
							}
							// every group after the target phrase is a new variant
							variants = append(variants, variant{
								name:    fields[k],
								output:  fields[k+1],
								variant: fields[k+2],
								line:    j,
							})
							k += 2
						}
					}
				}

				// for each variant, we need to create a new query
				for _, v := range variants {
					newQ := append([]string{}, q...)
					sections := strings.Fields(newQ[0])
					sections[2] = v.name
					sections[3] = v.output
					newQ[0] = strings.Join(sections, " ")
					newQ[v.line] = fmt.Sprintf("\t%s", v.variant)
					copies = append(copies, strings.Join(newQ, "\n"))
				}
			}
		}

		if len(copies) == 0 {
			continue
		}

		// write new queries to file
		f, err := os.Create(fmt.Sprintf("%s/%s.generated.sql", dir, strings.TrimSuffix(queryFile.Name(), ".sql")))
		if err != nil {
			return err
		}
		for _, q := range copies {
			_, err = f.WriteString(fmt.Sprintf("%s\n", q))
			if err != nil {
				_ = f.Close()
				return err
			}
		}
		_ = f.Close()
	}

	return nil
}

func readLines(p string) ([]string, error) {
	f, err := os.Open(p)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	lines := make([]string, 0)
	for sc.Scan() {
		lines = append(lines, sc.Text())
	}

	return lines, nil
}
