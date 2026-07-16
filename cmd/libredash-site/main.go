package main

import (
	"flag"
	"log"
	"net/http"

	sitehttp "github.com/Yacobolo/libredash/internal/site/http"
)

func main() {
	address := flag.String("addr", ":8081", "listen address")
	flag.Parse()

	log.Printf("LibreDash site listening on http://localhost%s", *address)
	if err := http.ListenAndServe(*address, sitehttp.NewHandler()); err != nil {
		log.Fatal(err)
	}
}
