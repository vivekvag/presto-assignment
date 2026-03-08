package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

func NewRouter(h *Handler) http.Handler {
	r := chi.NewRouter()
	r.Post("/api/v1/chargers", h.HandleCreateCharger)
	r.Put("/api/v1/chargers/{chargerID}/pricing", h.HandleUpdatePricing)
	r.Get("/api/v1/chargers/{chargerID}/pricing", h.HandleGetPricing)
	r.Put("/api/v1/pricing/bulk", h.HandleBulkUpdatePricing)
	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})
	return r
}
