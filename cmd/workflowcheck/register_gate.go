package main

import (
	"fmt"
	"net/http"
)

func runRegisterGate(client *apiClient) error {
	created, err := fixtureGate(client, "gate-east", "东侧泄洪闸", 4.5, 0.5)
	if err != nil {
		return err
	}
	status, err := stringField(created, "status")
	if err != nil {
		return err
	}
	if status != "idle" {
		return fmt.Errorf("created gate status: expected idle, got %q", status)
	}

	result, err := client.get("/gates/gate-east")
	if err != nil {
		return err
	}
	if err := requireStatus(result, http.StatusOK, "read registered gate"); err != nil {
		return err
	}
	fetched, err := objectField(result.Body, "gate")
	if err != nil {
		return err
	}
	name, err := stringField(fetched, "name")
	if err != nil {
		return err
	}
	if name != "东侧泄洪闸" {
		return fmt.Errorf("read gate name: expected %q, got %q", "东侧泄洪闸", name)
	}

	fmt.Println("register-gate: created and read gate-east")
	return nil
}
