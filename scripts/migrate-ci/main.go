package main

import (
	"database/sql"
	"fmt"

	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/cryptorand"
)

func main() {
	dbURL := "postgres://postgres:postgres@127.0.0.1:5432/postgres?sslmode=disable"
	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		panic(err)
	}
	defer db.Close()

	dbName, err := cryptorand.StringCharset(cryptorand.Lower, 10)
	if err != nil {
		panic(err)
	}

	dbName = "ci" + dbName
	_, err = db.Exec("CREATE DATABASE " + dbName)
	if err != nil {
		panic(err)
	}

	err = database.MigrateUp(db)
	if err != nil {
		panic(err)
	}

	_, _ = fmt.Println(dbName)
}
