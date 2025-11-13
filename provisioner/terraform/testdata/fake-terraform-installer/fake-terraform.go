package main

import "fmt"

var (
	version = ""

	output = fmt.Sprintf(`{
  "terraform_version": "%s",
  "platform": "linux_amd64",
  "provider_selections": {},
  "terraform_outdated": true
}`, version)
)

func main() {
	fmt.Println(output)
}
