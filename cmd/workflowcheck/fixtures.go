package main

import (
	"fmt"
	"net/http"
)

func fixtureGate(
	client *apiClient,
	id string,
	name string,
	maxAperture float64,
	initialAperture float64,
) (map[string]any, error) {
	result, err := client.post("/gates", map[string]any{
		"id":               id,
		"name":             name,
		"max_aperture":     maxAperture,
		"initial_aperture": initialAperture,
	})
	if err != nil {
		return nil, err
	}
	if err := requireStatus(result, http.StatusCreated, "create fixture gate"); err != nil {
		return nil, err
	}
	gate, err := objectField(result.Body, "gate")
	if err != nil {
		return nil, fmt.Errorf("create fixture gate response: %w", err)
	}
	return gate, nil
}

func fixtureCommand(
	client *apiClient,
	gateID string,
	targetAperture float64,
	reason string,
	requestedBy string,
) (map[string]any, error) {
	result, err := client.post("/commands", map[string]any{
		"gate_id":         gateID,
		"target_aperture": targetAperture,
		"reason":          reason,
		"requested_by":    requestedBy,
	})
	if err != nil {
		return nil, err
	}
	if err := requireStatus(result, http.StatusCreated, "create fixture command"); err != nil {
		return nil, err
	}
	command, err := objectField(result.Body, "command")
	if err != nil {
		return nil, fmt.Errorf("create fixture command response: %w", err)
	}
	return command, nil
}

func fixtureClearReview(
	client *apiClient,
	commandID string,
	verifier string,
) (map[string]any, error) {
	result, err := client.post("/commands/"+commandID+"/interlock", map[string]any{
		"verdict":  "clear",
		"verifier": verifier,
		"note":     "联锁条件满足",
	})
	if err != nil {
		return nil, err
	}
	if err := requireStatus(result, http.StatusOK, "clear fixture interlock"); err != nil {
		return nil, err
	}
	command, err := objectField(result.Body, "command")
	if err != nil {
		return nil, fmt.Errorf("clear fixture interlock response: %w", err)
	}
	return command, nil
}
