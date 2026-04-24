package main

import (
	"log"
	"os"

	"github.com/Spittingjiu/sui-go/internal/app"
)

func main() {
	addr := os.Getenv("ADDR")
	if addr == "" {
		addr = ":8811"
	}
	dbFile := os.Getenv("DB_FILE")
	if dbFile == "" {
		dbFile = "data/sui-go.db"
	}

	a, err := app.New(app.Config{
		Addr:      addr,
		DBFile:    dbFile,
		PanelUser: os.Getenv("PANEL_USER"),
		PanelPass: os.Getenv("PANEL_PASS"),
	})
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("sui-go listening on %s", addr)
	if err := a.Run(); err != nil {
		log.Fatal(err)
	}
}
