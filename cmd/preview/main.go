// Command preview serves web/ for a local look. It binds 127.0.0.1 only, on purpose, and has no host flag.
//
//	go run ./cmd/preview    # from the repo root, then open http://127.0.0.1:8080/agentbox-web/
package main

import (
	"log"
	"net/http"
	"strings"
)

const addr = "127.0.0.1:8080" // loopback only: this is a preview, never a deploy

const prefix = "/agentbox-web/" // the path of the live site, https://dbabi.github.io/agentbox-web/

func main() {
	// served under the same path as the live site, so the 404 page and every link work as they will there
	log.Printf("serving web/ on http://%s%s (ctrl+c stops)", addr, prefix)
	http.Handle(prefix, http.StripPrefix(strings.TrimSuffix(prefix, "/"), http.FileServer(http.Dir("web"))))
	log.Fatal(http.ListenAndServe(addr, nil))
}
