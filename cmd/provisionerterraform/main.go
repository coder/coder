package main

import (
	"context"
	"fmt"
	"os"

	"github.com/coder/coder/provisioner/terraform"
)

func main() {
	err := terraform.Serve(context.Background(), &terraform.ServeOptions{})
	if err != nil {
		_, _ = fmt.Println(err.Error())
		os.Exit(1)
	}
}
