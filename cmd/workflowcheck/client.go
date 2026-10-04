package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type apiClient struct {
	baseURL string
	client  *http.Client
}

type apiResult struct {
	Status int
	Body   map[string]any
}

func newAPIClient(baseURL string) *apiClient {
	return &apiClient{
		baseURL: strings.TrimRight(baseURL, "/"),
		client:  &http.Client{Timeout: 5 * time.Second},
	}
}

func (c *apiClient) get(path string) (apiResult, error) {
	return c.do(http.MethodGet, path, nil)
}

func (c *apiClient) post(path string, payload any) (apiResult, error) {
	return c.do(http.MethodPost, path, payload)
}

func (c *apiClient) do(method, path string, payload any) (apiResult, error) {
	var body io.Reader
	if payload != nil {
		var buffer bytes.Buffer
		if err := json.NewEncoder(&buffer).Encode(payload); err != nil {
			return apiResult{}, fmt.Errorf("encode request: %w", err)
		}
		body = &buffer
	}

	request, err := http.NewRequest(method, c.baseURL+path, body)
	if err != nil {
		return apiResult{}, fmt.Errorf("build request: %w", err)
	}
	request.Header.Set("Accept", "application/json")
	if payload != nil {
		request.Header.Set("Content-Type", "application/json")
	}

	response, err := c.client.Do(request)
	if err != nil {
		return apiResult{}, fmt.Errorf("call %s %s: %w", method, path, err)
	}
	defer response.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return apiResult{}, fmt.Errorf("read %s %s response: %w", method, path, err)
	}

	result := apiResult{
		Status: response.StatusCode,
		Body:   map[string]any{},
	}
	if len(bytes.TrimSpace(raw)) == 0 {
		return result, nil
	}
	if err := json.Unmarshal(raw, &result.Body); err != nil {
		return apiResult{}, fmt.Errorf("decode %s %s response: %w", method, path, err)
	}
	return result, nil
}

func requireStatus(result apiResult, expected int, operation string) error {
	if result.Status == expected {
		return nil
	}
	return fmt.Errorf(
		"%s: expected HTTP %d, got %d with body %s",
		operation,
		expected,
		result.Status,
		prettyBody(result.Body),
	)
}

func objectField(body map[string]any, name string) (map[string]any, error) {
	value, exists := body[name]
	if !exists {
		return nil, fmt.Errorf("response is missing object field %q", name)
	}
	object, ok := value.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("response field %q is not an object", name)
	}
	return object, nil
}

func stringField(object map[string]any, name string) (string, error) {
	value, exists := object[name]
	if !exists {
		return "", fmt.Errorf("object is missing string field %q", name)
	}
	text, ok := value.(string)
	if !ok {
		return "", fmt.Errorf("object field %q is not a string", name)
	}
	return text, nil
}

func floatField(object map[string]any, name string) (float64, error) {
	value, exists := object[name]
	if !exists {
		return 0, fmt.Errorf("object is missing numeric field %q", name)
	}
	number, ok := value.(float64)
	if !ok {
		return 0, fmt.Errorf("object field %q is not a number", name)
	}
	return number, nil
}

func prettyBody(body map[string]any) string {
	encoded, err := json.Marshal(body)
	if err != nil {
		return "<unavailable>"
	}
	return string(encoded)
}
