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

func modulesRouter(database *db.Database) func(chi.Router) {
	return func(r chi.Router) {
		r.Get("/", listModules(database))
		r.Post("/", createModule(database))
		r.Get("/{id}", getModule(database))
		r.Delete("/{id}", deleteModule(database))
	}
}

func listModules(database *db.Database) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		esphomeOnly := r.URL.Query().Get("esphome") == "true"
		mods, err := database.ListModules()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		type moduleResp struct {
			db.ModuleRow
			HasESPHome bool `json:"has_esphome"`
		}
		var results []moduleResp
		for _, m := range mods {
			mod, parseErr := yamldef.ParseModule([]byte(m.YAMLBody))
			hasESPHome := parseErr == nil && mod.ESPHome != nil
			if esphomeOnly && !hasESPHome {
				continue
			}
			results = append(results, moduleResp{ModuleRow: m, HasESPHome: hasESPHome})
		}
		if results == nil {
			results = []moduleResp{}
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(results)
	}
}

func createModule(database *db.Database) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			ID       string `json:"id"`
			Name     string `json:"name"`
			Category string `json:"category"`
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
		if _, err := yamldef.ParseModule([]byte(req.YAMLBody)); err != nil {
			http.Error(w, "invalid module YAML: "+err.Error(), http.StatusUnprocessableEntity)
			return
		}
		if err := database.CreateModule(db.ModuleRow{
			ID: req.ID, Name: req.Name, Category: req.Category, YAMLBody: req.YAMLBody,
		}); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusCreated)
	}
}

func getModule(database *db.Database) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		m, err := database.GetModule(id)
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(m)
	}
}

func deleteModule(database *db.Database) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		if err := database.DeleteModule(id); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}
