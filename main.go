package main

import (
	"flag"
	"log"
	"net/http"
	"os"

	"task035-csvjson/internal/httpapi"
	"task035-csvjson/internal/selfcheck"
)

func main() {
	log.SetFlags(0)

	if len(os.Args) > 1 && os.Args[1] == "--smoke-test" {
		os.Exit(selfcheck.Run())
	}

	// Default server mode; tolerate an optional leading "server" subcommand.
	args := os.Args[1:]
	if len(args) > 0 && args[0] == "server" {
		args = args[1:]
	}
	addr := ":8080"
	fs := flag.NewFlagSet("server", flag.ExitOnError)
	fs.StringVar(&addr, "addr", addr, "listen address")
	_ = fs.Parse(args)

	srv := httpapi.New()
	log.Printf("listening on %s", addr)
	log.Fatal(http.ListenAndServe(addr, srv.Handler()))
}
