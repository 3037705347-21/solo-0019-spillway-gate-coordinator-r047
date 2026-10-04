package bootstrap

import (
	"net/http"

	"example.com/spillway-gate-coordinator/internal/coordination"
	"example.com/spillway-gate-coordinator/internal/httpapi"
	"example.com/spillway-gate-coordinator/internal/storage"
)

func NewHandler() http.Handler {
	repository := storage.NewMemoryRepository()
	service := coordination.NewService(repository)
	return httpapi.NewRouter(service)
}
