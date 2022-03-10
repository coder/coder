package template

import (
	"embed"
	"fmt"
)

//go:embed */*.json
//go:embed */*.tf
var files embed.FS

func List() {
	dirs, err := files.ReadDir(".")
	if err != nil {
		panic(err)
	}
	fmt.Printf("Dirs: %+v\n", dirs)
	for _, dir := range dirs {
		subpaths, err := files.ReadDir(dir.Name())
		if err != nil {
			panic(err)
		}
		for _, subpath := range subpaths {
			fmt.Printf("Got dir: %s\n", subpath.Name())
		}
	}
}

func Setup() {

}
