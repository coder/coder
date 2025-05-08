package main

import (
	"fmt"
	"os"

	"github.com/coder/coder/v2/coderd/database/dbtestutil/dbpool"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: dbpoolclient <command> [args]")
		fmt.Println("Commands:")
		fmt.Println("  getdb")
		fmt.Println("  dispose <db_url>")
		os.Exit(1)
	}

	client, err := dbpool.NewClient("localhost:8080")
	if err != nil {
		panic(err)
	}

	command := os.Args[1]

	switch command {
	case "getdb":
		if len(os.Args) != 2 {
			fmt.Println("Usage: dbpoolclient getdb")
			os.Exit(1)
		}
		fmt.Println("getting db")
		dbURL, err := client.GetDB()
		if err != nil {
			panic(err)
		}
		fmt.Println(dbURL)
	case "dispose":
		if len(os.Args) != 3 {
			fmt.Println("Usage: dbpoolclient dispose <db_url>")
			os.Exit(1)
		}
		dbURL := os.Args[2]
		fmt.Printf("disposing db: %s\n", dbURL)
		err := client.DisposeDB(dbURL)
		if err != nil {
			panic(err)
		}
		fmt.Println("db disposed successfully")
	default:
		fmt.Printf("Unknown command: %s\n", command)
		os.Exit(1)
	}
}
