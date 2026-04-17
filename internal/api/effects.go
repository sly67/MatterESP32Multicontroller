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

func effectsRouter(database *db.Database) func(chi.Router) {
	return func(r chi.Router) {
		r.Get("/", listEffects(database))
		r.Post("/", createEffect(database))
		r.Get("/{id}", getEffect(database))
		r.Delete("/{id}", deleteEffect(database))
	}
}

func listEffects(database *db.Database) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		effs, err := database.ListEffects()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if effs == nil {
			effs = []db.EffectRow{}
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(effs)
	}
}

func createEffect(database *db.Database) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			ID       string `json:"id"`
			Name     string `json:"name"`
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
		eff, err := yamldef.ParseEffect([]byte(req.YAMLBody))
		if err != nil {
			http.Error(w, "invalid effect YAML: "+err.Error(), http.StatusUnprocessableEntity)
			return
		}
		if err := database.CreateEffect(db.EffectRow{
			ID: req.ID, Name: eff.Name, YAMLBody: req.YAMLBody,
		}); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusCreated)
	}
}

func getEffect(database *db.Database) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		e, err := database.GetEffect(id)
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(e)
	}
}

func deleteEffect(database *db.Database) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		if err := database.DeleteEffect(id); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}
