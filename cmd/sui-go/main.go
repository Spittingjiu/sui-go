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
	dataFile := os.Getenv("DATA_FILE")
	if dataFile == "" {
		dataFile = "data/inbounds.json"
	}

	a, err := app.New(app.Config{
		Addr:     addr,
		DataFile: dataFile,
	})
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("sui-go listening on %s", addr)
	if err := a.Run(); err != nil {
		log.Fatal(err)
	}
}
