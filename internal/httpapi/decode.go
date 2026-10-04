package httpapi

import (
	"encoding/json"
	"io"
	"net/http"

	"example.com/spillway-gate-coordinator/internal/contracts"
	"example.com/spillway-gate-coordinator/internal/domain"
)

const maxRequestBodyBytes = 1 << 20

func decodeJSON(w http.ResponseWriter, r *http.Request, target any) error {
	body := http.MaxBytesReader(w, r.Body, maxRequestBodyBytes)
	decoder := json.NewDecoder(body)
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(target); err != nil {
		return domain.Invalid("invalid_json", "request body must be a single valid JSON object")
	}

	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return domain.Invalid("invalid_json", "request body must contain only one JSON object")
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeDomainError(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	switch domain.ErrorKindOf(err) {
	case domain.KindInvalid:
		status = http.StatusBadRequest
	case domain.KindNotFound:
		status = http.StatusNotFound
	case domain.KindConflict:
		status = http.StatusConflict
	}

	response := contracts.ErrorResponse{
		Code:    domain.ErrorCodeOf(err),
		Message: domain.ErrorMessageOf(err),
	}
	writeJSON(w, status, response)
}
