package main

import (
	"database/sql"
	"fmt"

	"github.com/coder/coder/v2/coderd/database/migrations"
	"github.com/coder/coder/v2/cryptorand"
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

	targetURL := fmt.Sprintf("postgres://postgres:postgres@127.0.0.1:5432/%s?sslmode=disable", dbName)
	target, err := sql.Open("postgres", targetURL)
	if err != nil {
		panic(err)
	}
	defer target.Close()

	err = migrations.Up(target)
	if err != nil {
		panic(err)
	}

	_, _ = fmt.Println(dbName)
}
