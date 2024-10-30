package main

import (
	"os"
	"path/filepath"

	embeddedpostgres "github.com/fergusstrange/embedded-postgres"
)

func main() {
	postgresPath := filepath.Join(os.TempDir(), "coder-test-postgres")
	ep := embeddedpostgres.NewDatabase(
		embeddedpostgres.DefaultConfig().
			Version(embeddedpostgres.V16).
			BinariesPath(filepath.Join(postgresPath, "bin")).
			DataPath(filepath.Join(postgresPath, "data")).
			RuntimePath(filepath.Join(postgresPath, "runtime")).
			CachePath(filepath.Join(postgresPath, "cache")).
			Username("postgres").
			Password("postgres").
			Database("postgres").
			Port(uint32(5432)).
			StartParameters(map[string]string{"max_connections": "1000"}).
			Logger(os.Stdout),
	)
	err := ep.Start()
	if err != nil {
		panic(err)
	}
}
