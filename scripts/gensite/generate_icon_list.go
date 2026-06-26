package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/coder/coder/v2/scripts/atomicwrite"
)

func generateIconList(path string) int {
	if path == "" {
		return 0 // skip
	}

	files, err := os.ReadDir("site/static/icon/")
	if err != nil {
		_, _ = fmt.Println("failed to read site/static/icon/ directory")
		_, _ = fmt.Println("err:", err.Error())
		return 71 // OSERR
	}

	icons := make([]string, len(files))
	i := 0
	for _, file := range files {
		if !file.Type().IsRegular() {
			continue
		}

		icons[i] = file.Name()
		i++
	}
	icons = icons[:i]

	iconsJSON, err := json.Marshal(icons)
	if err != nil {
		_, _ = fmt.Println("failed to serialize JSON")
		_, _ = fmt.Println("err:", err.Error())
		return 70 // SOFTWARE
	}

	if err := atomicwrite.File(path, iconsJSON); err != nil {
		_, _ = fmt.Println("failed to write JSON")
		_, _ = fmt.Println("err:", err.Error())
		return 74 // IOERR
	}

	_, _ = fmt.Println(green.Sprintf("==>"), path)

	return 0
}
