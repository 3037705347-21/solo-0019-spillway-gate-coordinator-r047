package httpapi

import (
	"net/http"

	"example.com/spillway-gate-coordinator/internal/contracts"
)

func (h *handler) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *handler) registerGate(w http.ResponseWriter, r *http.Request) {
	var request contracts.RegisterGateRequest
	if err := decodeJSON(w, r, &request); err != nil {
		writeDomainError(w, err)
		return
	}

	gate, err := h.service.RegisterGate(r.Context(), request)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, contracts.RegisterGateResponse{Gate: gateView(gate)})
}

func (h *handler) gate(w http.ResponseWriter, r *http.Request) {
	gate, err := h.service.Gate(r.Context(), r.PathValue("id"))
	if err != nil {
		writeDomainError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, contracts.GateResponse{Gate: gateView(gate)})
}

func (h *handler) requestCommand(w http.ResponseWriter, r *http.Request) {
	var request contracts.RequestCommandRequest
	if err := decodeJSON(w, r, &request); err != nil {
		writeDomainError(w, err)
		return
	}

	command, err := h.service.RequestCommand(r.Context(), request)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	writeJSON(
		w,
		http.StatusCreated,
		contracts.CommandResponse{Command: commandView(command, false)},
	)
}

func (h *handler) command(w http.ResponseWriter, r *http.Request) {
	command, err := h.service.Command(r.Context(), r.PathValue("id"))
	if err != nil {
		writeDomainError(w, err)
		return
	}
	writeJSON(
		w,
		http.StatusOK,
		contracts.CommandResponse{Command: commandView(command, false)},
	)
}

func (h *handler) expireReviewWindows(w http.ResponseWriter, r *http.Request) {
	expiredCommandIDs, err := h.service.ExpireReviewWindows(r.Context())
	if err != nil {
		writeDomainError(w, err)
		return
	}
	writeJSON(
		w,
		http.StatusOK,
		contracts.ExpireReviewWindowsResponse{
			ExpiredCommandIDs: expiredCommandIDs,
		},
	)
}

func (h *handler) reviewInterlock(w http.ResponseWriter, r *http.Request) {
	var request contracts.ReviewInterlockRequest
	if err := decodeJSON(w, r, &request); err != nil {
		writeDomainError(w, err)
		return
	}

	command, gate, err := h.service.ReviewInterlock(r.Context(), r.PathValue("id"), request)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	writeJSON(
		w,
		http.StatusOK,
		contracts.ReviewCommandResponse{
			Command: commandView(command, true),
			Gate:    gateView(gate),
		},
	)
}

func (h *handler) executeCommand(w http.ResponseWriter, r *http.Request) {
	var request contracts.ExecuteCommandRequest
	if err := decodeJSON(w, r, &request); err != nil {
		writeDomainError(w, err)
		return
	}

	command, gate, err := h.service.ExecuteCommand(r.Context(), r.PathValue("id"), request)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	writeJSON(
		w,
		http.StatusOK,
		contracts.ExecuteCommandResponse{
			Command: commandView(command, false),
			Gate:    gateView(gate),
		},
	)
}
