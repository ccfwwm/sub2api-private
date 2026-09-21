package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"time"
)

// ImageDeploymentRequest accepts release identifiers only. Docker commands and
// image references are resolved and validated by the host controller.
type ImageDeploymentRequest struct {
	Action  string `json:"action"`
	Version string `json:"version,omitempty"`
	JobID   string `json:"job_id,omitempty"`
	Confirm bool   `json:"confirm"`
}

// CallImageDeployment uses a local Unix socket, never a user-supplied URL or
// Docker socket. Only the host controller has permission to change containers.
func CallImageDeployment(ctx context.Context, request *ImageDeploymentRequest) (json.RawMessage, error) {
	path := os.Getenv("IMAGE_DEPLOY_SOCKET")
	if path == "" {
		return nil, fmt.Errorf("image deployment is not installed; production changes require operator approval")
	}
	transport := &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{Timeout: 5 * time.Second}).DialContext(ctx, "unix", path)
	}}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 15 * time.Second}
	method := http.MethodGet
	var body io.Reader
	if request != nil {
		method = http.MethodPost
		data, err := json.Marshal(request)
		if err != nil {
			return nil, err
		}
		body = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, "http://localhost/deployment", body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("host deployment service is unavailable")
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 1024*1024+1))
	if err != nil || len(data) > 1024*1024 || !json.Valid(data) {
		return nil, fmt.Errorf("invalid host deployment response")
	}
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
		var result struct {
			Error string `json:"error"`
		}
		_ = json.Unmarshal(data, &result)
		return nil, fmt.Errorf("deployment rejected: %s", result.Error)
	}
	return json.RawMessage(data), nil
}
