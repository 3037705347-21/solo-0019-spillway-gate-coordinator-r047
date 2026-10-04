package httpapi

import (
	"net/http"

	"example.com/spillway-gate-coordinator/internal/coordination"
)

type handler struct {
	service *coordination.Service
}

func NewRouter(service *coordination.Service) http.Handler {
	h := &handler{service: service}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", h.health)
	mux.HandleFunc("POST /gates", h.registerGate)
	mux.HandleFunc("GET /gates/{id}", h.gate)
	mux.HandleFunc("POST /commands", h.requestCommand)
	mux.HandleFunc("GET /commands/{id}", h.command)
	mux.HandleFunc("POST /maintenance/review-windows/expire", h.expireReviewWindows)
	mux.HandleFunc("POST /commands/{id}/interlock", h.reviewInterlock)
	mux.HandleFunc("POST /commands/{id}/execute", h.executeCommand)
	return recoverMiddleware(mux)
}

func recoverMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if recover() != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{
					"code":    "internal_error",
					"message": "internal server error",
				})
			}
		}()
		next.ServeHTTP(w, r)
	})
}
