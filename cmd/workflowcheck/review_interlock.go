package main

import (
	"fmt"
	"net/http"
)

func runReviewSafetyInterlock(client *apiClient) error {
	if _, err := fixtureGate(client, "gate-south", "南侧泄洪闸", 5, 0.25); err != nil {
		return err
	}
	first, err := fixtureCommand(client, "gate-south", 1.5, "下游流量调整", "dispatcher-yang")
	if err != nil {
		return err
	}
	firstID, err := stringField(first, "id")
	if err != nil {
		return err
	}

	approved, err := fixtureClearReview(client, firstID, "safety-wu")
	if err != nil {
		return err
	}
	status, err := stringField(approved, "status")
	if err != nil {
		return err
	}
	if status != "approved" {
		return fmt.Errorf("clear review status: expected approved, got %q", status)
	}
	token, err := stringField(approved, "execution_token")
	if err != nil {
		return err
	}
	if token == "" {
		return fmt.Errorf("clear review did not issue an execution token")
	}

	second, err := fixtureCommand(client, "gate-south", 2.5, "第二条冲突命令", "dispatcher-yang")
	if err != nil {
		return err
	}
	secondID, err := stringField(second, "id")
	if err != nil {
		return err
	}
	conflict, err := client.post("/commands/"+secondID+"/interlock", map[string]any{
		"verdict":  "clear",
		"verifier": "safety-wu",
		"note":     "并发占用验证",
	})
	if err != nil {
		return err
	}
	if err := requireStatus(conflict, http.StatusConflict, "reject second gate reservation"); err != nil {
		return err
	}

	gateResult, err := client.get("/gates/gate-south")
	if err != nil {
		return err
	}
	if err := requireStatus(gateResult, http.StatusOK, "read reserved gate"); err != nil {
		return err
	}
	gate, err := objectField(gateResult.Body, "gate")
	if err != nil {
		return err
	}
	gateStatus, err := stringField(gate, "status")
	if err != nil {
		return err
	}
	activeCommandID, err := stringField(gate, "active_command_id")
	if err != nil {
		return err
	}
	if gateStatus != "reserved" || activeCommandID != firstID {
		return fmt.Errorf(
			"gate reservation mismatch: status=%q active_command_id=%q",
			gateStatus,
			activeCommandID,
		)
	}

	fmt.Println("review-safety-interlock: first clear review reserved gate; conflicting review rejected")
	return nil
}
