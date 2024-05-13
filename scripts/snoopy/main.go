package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/fatih/color"
)

var green = color.New(color.FgGreen).Add(color.Bold)

func main() {
	token := os.Args[1]

	for {
		req, err := http.NewRequest("GET", "http://localhost:3111/api/v2/workspaceagents/me/metadata", nil)
		if err != nil {
			panic("uh oh")
		}
		req.Header.Set("Coder-Session-Token", token)

		res, err := http.DefaultClient.Do(req)
		if err != nil {
			panic("oh no")
		}
		defer res.Body.Close()

		buf, err := io.ReadAll(res.Body)
		if err != nil {
			panic("geez")
		}

		_, _ = fmt.Print(time.Now())
		_, err = os.Stdout.Write(buf)
		if err != nil {
			panic("geez heck")
		}
		print("\n\n\n")

		<-time.After(time.Second)
	}
}
