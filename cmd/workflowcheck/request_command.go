package main

import (
	"fmt"
	"net/http"
)

func runRequestCommand(client *apiClient) error {
	if _, err := fixtureGate(client, "gate-north", "北侧泄洪闸", 6, 0.5); err != nil {
		return err
	}
	command, err := fixtureCommand(
		client,
		"gate-north",
		2.75,
		"上游水位调整",
		"dispatcher-lin",
	)
	if err != nil {
		return err
	}

	status, err := stringField(command, "status")
	if err != nil {
		return err
	}
	if status != "pending" {
		return fmt.Errorf("new command status: expected pending, got %q", status)
	}
	startingAperture, err := floatField(command, "starting_aperture")
	if err != nil {
		return err
	}
	if startingAperture != 0.5 {
		return fmt.Errorf("starting aperture: expected 0.5, got %v", startingAperture)
	}

	invalid, err := client.post("/commands", map[string]any{
		"gate_id":         "gate-north",
		"target_aperture": 8.0,
		"reason":          "越界测试",
		"requested_by":    "dispatcher-lin",
	})
	if err != nil {
		return err
	}
	if err := requireStatus(invalid, http.StatusBadRequest, "reject out-of-range command"); err != nil {
		return err
	}

	fmt.Println("request-gate-command: pending command stored and invalid target rejected")
	return nil
}
