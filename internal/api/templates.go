package api

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/karthangar/matteresp32hub/internal/db"
	"github.com/karthangar/matteresp32hub/internal/yamldef"
)

func templatesRouter(database *db.Database) func(chi.Router) {
	return func(r chi.Router) {
		r.Get("/", listTemplates(database))
		r.Post("/", createTemplate(database))
		r.Get("/{id}", getTemplate(database))
		r.Delete("/{id}", deleteTemplate(database))
	}
}

func listTemplates(database *db.Database) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tpls, err := database.ListTemplates()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if tpls == nil {
			tpls = []db.TemplateRow{}
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(tpls)
	}
}

func createTemplate(database *db.Database) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			ID       string `json:"id"`
			Name     string `json:"name"`
			Board    string `json:"board"`
			YAMLBody string `json:"yaml_body"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid JSON", http.StatusBadRequest)
			return
		}
		if req.ID == "" || req.YAMLBody == "" {
			http.Error(w, "id and yaml_body are required", http.StatusBadRequest)
			return
		}
		tpl, err := yamldef.ParseTemplate([]byte(req.YAMLBody))
		if err != nil {
			http.Error(w, "invalid template YAML: "+err.Error(), http.StatusUnprocessableEntity)
			return
		}
		if err := database.CreateTemplate(db.TemplateRow{
			ID: req.ID, Name: req.Name, Board: tpl.Board, YAMLBody: req.YAMLBody,
		}); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusCreated)
	}
}

func getTemplate(database *db.Database) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		t, err := database.GetTemplate(id)
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(t)
	}
}

func deleteTemplate(database *db.Database) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		if err := database.DeleteTemplate(id); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}
