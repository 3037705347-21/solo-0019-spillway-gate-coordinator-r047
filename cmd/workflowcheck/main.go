package main

import (
	"flag"
	"fmt"
	"net/http/httptest"
	"os"

	"example.com/spillway-gate-coordinator/internal/bootstrap"
)

func main() {
	workflow := flag.String("workflow", "", "workflow id to verify")
	flag.Parse()
	if *workflow == "" {
		fmt.Fprintln(os.Stderr, "missing --workflow")
		flag.Usage()
		os.Exit(2)
	}

	server := httptest.NewServer(bootstrap.NewHandler())
	defer server.Close()
	client := newAPIClient(server.URL)

	var err error
	switch *workflow {
	case "register-gate":
		err = runRegisterGate(client)
	case "request-gate-command":
		err = runRequestCommand(client)
	case "review-safety-interlock":
		err = runReviewSafetyInterlock(client)
	case "execute-gate-command":
		err = runExecuteGateCommand(client)
	default:
		fmt.Fprintf(os.Stderr, "unknown workflow: %s\n", *workflow)
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "workflow %s failed: %v\n", *workflow, err)
		os.Exit(1)
	}
}
