package main

import (
	"fmt"
	"os"

	db "forge/internal/db"
)

func main() {
	const path = "dev/forge-dev.db"

	_ = os.Remove(path)

	database, err := db.Open(path)
	if err != nil {
		panic(err)
	}
	defer database.Close()

	if err := database.Migrate(); err != nil {
		panic(err)
	}

	fmt.Println("dev db ready:", path)
}
