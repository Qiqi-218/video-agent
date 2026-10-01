// Package httpapi exposes the same constrained P3 tools over loopback HTTP.
package httpapi

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/zylar06/video-agent/internal/agent"
	"github.com/zylar06/video-agent/internal/app"
)

func New(a *app.App) http.Handler {
	s := agent.NewService(a)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/health", func(w http.ResponseWriter, r *http.Request) {
		write(w, http.StatusOK, agent.Envelope{APIVersion: agent.APIVersion, OK: true, Result: map[string]string{"status": "ok"}})
	})
	mux.HandleFunc("GET /v1/tools", func(w http.ResponseWriter, r *http.Request) {
		write(w, http.StatusOK, agent.Envelope{APIVersion: agent.APIVersion, OK: true, Result: s.Names()})
	})
	mux.HandleFunc("POST /v1/tools/{name}", func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		body, err := io.ReadAll(io.LimitReader(r.Body, 4<<20))
		if err != nil {
			write(w, http.StatusBadRequest, invalid(err))
			return
		}
		result := s.Call(r.Context(), r.PathValue("name"), body)
		write(w, status(result), result)
	})
	mux.HandleFunc("GET /v1/jobs/{id}", func(w http.ResponseWriter, r *http.Request) {
		j, err := a.Store.Job(r.PathValue("id"))
		if err != nil {
			write(w, http.StatusNotFound, agent.Envelope{APIVersion: agent.APIVersion, OK: false, Error: &agent.APIError{Code: "not_found", Message: err.Error()}})
			return
		}
		write(w, http.StatusOK, agent.Envelope{APIVersion: agent.APIVersion, OK: true, Result: j})
	})
	mux.HandleFunc("POST /v1/jobs/{id}/cancel", func(w http.ResponseWriter, r *http.Request) {
		body, _ := json.Marshal(map[string]string{"id": r.PathValue("id")})
		result := s.Call(r.Context(), "jobs_cancel", body)
		write(w, status(result), result)
	})
	mux.HandleFunc("GET /v1/artifacts/{id}", func(w http.ResponseWriter, r *http.Request) {
		j, err := a.Store.Job(r.PathValue("id"))
		if err != nil || j.Status != "completed" {
			write(w, http.StatusNotFound, agent.Envelope{APIVersion: agent.APIVersion, OK: false, Error: &agent.APIError{Code: "not_found", Message: "completed artifact not found"}})
			return
		}
		base := filepath.Join(a.Store.Dir, "exports")
		rel, err := filepath.Rel(base, j.Output)
		if err != nil || rel == "." || strings.HasPrefix(rel, "..") || filepath.IsAbs(rel) {
			write(w, http.StatusForbidden, agent.Envelope{APIVersion: agent.APIVersion, OK: false, Error: &agent.APIError{Code: "invalid_request", Message: "artifact is outside managed exports"}})
			return
		}
		f, err := os.Open(j.Output)
		if err != nil {
			write(w, http.StatusNotFound, agent.Envelope{APIVersion: agent.APIVersion, OK: false, Error: &agent.APIError{Code: "not_found", Message: "artifact file not found"}})
			return
		}
		defer f.Close()
		info, err := f.Stat()
		if err != nil || !info.Mode().IsRegular() {
			write(w, http.StatusNotFound, agent.Envelope{APIVersion: agent.APIVersion, OK: false, Error: &agent.APIError{Code: "not_found", Message: "artifact file not found"}})
			return
		}
		http.ServeContent(w, r, filepath.Base(j.Output), info.ModTime(), f)
	})
	return securityHeaders(mux)
}

func Server(addr string, h http.Handler) *http.Server {
	return &http.Server{Addr: addr, Handler: h, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 30 * time.Second, WriteTimeout: 0, IdleTimeout: 60 * time.Second}
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		next.ServeHTTP(w, r)
	})
}
func invalid(err error) agent.Envelope {
	return agent.Envelope{APIVersion: agent.APIVersion, OK: false, Error: &agent.APIError{Code: "invalid_request", Message: err.Error()}}
}
func status(e agent.Envelope) int {
	if e.OK {
		return http.StatusOK
	}
	if e.Error != nil && e.Error.Code == "not_found" {
		return http.StatusNotFound
	}
	if e.Error != nil && e.Error.Code == "revision_conflict" {
		return http.StatusConflict
	}
	return http.StatusBadRequest
}
func write(w http.ResponseWriter, code int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(body)
}
