// Command assessd is P1, the assessment engine (CLAUDE.md §7): sync, pure, fast.
// ingest -> core -> API/MCP. No credentials. No external calls.
//
// PC-21: /assess (HTTP) and the "assess" MCP tool (mounted at /mcp, streamable HTTP
// transport) both live here, backed by the same server.Store and the same
// server.Assess function — see server/http.go, server/mcp.go.
//
// PC-82: /simulate lives here too, now that it has its own real specification (PC-21's
// own title named it prematurely, with no acceptance criteria of its own at the time —
// that gap is what PC-82 exists to close, not scope invented here).
package main

import (
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"preflight/server"
)

func main() {
	dbPath := os.Getenv("PREFLIGHT_DB_PATH")
	if dbPath == "" {
		dbPath = "preflight.db"
	}
	store, err := server.OpenStore(dbPath)
	if err != nil {
		log.Fatalf("assessd: open store: %v", err)
	}
	defer store.Close()

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "assessd (P1): ok — no credentials held, no external calls made")
	})
	mux.HandleFunc("/assess", server.AssessHandler(store))
	mux.HandleFunc("/simulate", server.SimulateHandler(store))
	mux.HandleFunc("POST /sessions/{id}/canvas", server.AssessCanvasHandler(store))
	mux.HandleFunc("GET /sessions/{id}/versions/{n}", server.GetVersionHandler(store))
	mux.HandleFunc("/openapi.json", server.OpenAPISpecHandler())
	mux.HandleFunc("/swagger", server.SwaggerUIHandler())

	mcpServer := server.NewMCPServer(store)
	mcpHandler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return mcpServer }, nil)
	mux.Handle("/mcp", mcpHandler)

	port := os.Getenv("PREFLIGHT_ASSESSD_PORT")
	if port == "" {
		port = "8080"
	}
	addr := ":" + port
	log.Printf("assessd (P1) listening on %s — deterministic assessment engine, no credentials", addr)
	log.Printf("assessd (P1): SQLite store at %s (ADR-002)", dbPath)
	log.Fatal(http.ListenAndServe(addr, server.WithCORS(mux)))
}
