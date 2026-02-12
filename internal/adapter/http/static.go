package http

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed web/*
var webFS embed.FS

func (h *Handler) ServeStatic(mux *http.ServeMux) {
	subFS, _ := fs.Sub(webFS, "web")
	mux.Handle("GET /", http.FileServer(http.FS(subFS)))
}
