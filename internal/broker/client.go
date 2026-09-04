package broker

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"

	"github.com/Starlight-Unit-Studio/Quantum-Control/internal/protocol"
	"github.com/Starlight-Unit-Studio/Quantum-Control/internal/security"
)

const maxBrokerResponseBytes = int64(4 << 20)

type API interface {
	Health(context.Context) error
	Catalog(context.Context) ([]protocol.OperationDefinition, error)
	Plan(context.Context, protocol.OperationRequest) (protocol.OperationPlan, error)
	Execute(context.Context, protocol.OperationRequest) (protocol.OperationResponse, error)
}

type Client struct {
	httpClient *http.Client
	token      string
}

func NewClient(socketPath, token string, timeout time.Duration) *Client {
	transport := &http.Transport{
		DisableCompression: true,
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			var dialer net.Dialer
			return dialer.DialContext(ctx, "unix", socketPath)
		},
	}
	return &Client{httpClient: &http.Client{Transport: transport, Timeout: timeout}, token: token}
}

func (c *Client) Health(ctx context.Context) error {
	var response map[string]any
	return c.do(ctx, http.MethodGet, "/healthz", nil, &response, false, http.StatusOK)
}

func (c *Client) Catalog(ctx context.Context) ([]protocol.OperationDefinition, error) {
	var response struct {
		Operations []protocol.OperationDefinition `json:"operations"`
	}
	if err := c.do(ctx, http.MethodGet, "/v1/operations", nil, &response, true, http.StatusOK); err != nil {
		return nil, err
	}
	return response.Operations, nil
}

func (c *Client) Plan(ctx context.Context, request protocol.OperationRequest) (protocol.OperationPlan, error) {
	var response protocol.OperationPlan
	err := c.do(ctx, http.MethodPost, "/v1/plan", request, &response, true, http.StatusOK, http.StatusBadRequest)
	return response, err
}

func (c *Client) Execute(ctx context.Context, request protocol.OperationRequest) (protocol.OperationResponse, error) {
	var response protocol.OperationResponse
	err := c.do(ctx, http.MethodPost, "/v1/execute", request, &response, true, http.StatusOK, http.StatusBadRequest, http.StatusInternalServerError)
	return response, err
}

func (c *Client) Confirm(ctx context.Context, plan security.OperationPlan, actorToken string) (security.GrantResponse, error) {
	var response security.GrantResponse
	err := c.do(ctx, http.MethodPost, "/v1/confirm", confirmationEnvelope{Plan: plan, ActorToken: actorToken}, &response, true, http.StatusCreated)
	return response, err
}

func (c *Client) ExecuteApproved(ctx context.Context, plan security.OperationPlan, confirmationToken string, actorToken string) (protocol.OperationResponse, error) {
	var response protocol.OperationResponse
	err := c.do(ctx, http.MethodPost, "/v1/execute-approved", approvedExecutionEnvelope{Plan: plan, ConfirmationToken: confirmationToken, ActorToken: actorToken}, &response, true, http.StatusOK, http.StatusBadRequest, http.StatusInternalServerError)
	return response, err
}

func (c *Client) do(ctx context.Context, method string, path string, input any, output any, authenticated bool, acceptedStatuses ...int) error {
	var body io.Reader
	if input != nil {
		encoded, err := json.Marshal(input)
		if err != nil {
			return fmt.Errorf("encode broker request: %w", err)
		}
		body = bytes.NewReader(encoded)
	}
	req, err := http.NewRequestWithContext(ctx, method, "http://qcored"+path, body)
	if err != nil {
		return fmt.Errorf("create broker request: %w", err)
	}
	if input != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if authenticated {
		req.Header.Set("X-Quantum-Broker-Token", c.token)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("broker request: %w", err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxBrokerResponseBytes+1))
	if err != nil {
		return fmt.Errorf("read broker response: %w", err)
	}
	if int64(len(data)) > maxBrokerResponseBytes {
		return fmt.Errorf("broker response exceeds %d bytes", maxBrokerResponseBytes)
	}
	if !containsStatus(acceptedStatuses, resp.StatusCode) {
		message := bytes.TrimSpace(data)
		if len(message) > 64<<10 {
			message = message[:64<<10]
		}
		return fmt.Errorf("broker returned HTTP %d: %s", resp.StatusCode, message)
	}
	if output == nil {
		return nil
	}
	if err := json.Unmarshal(data, output); err != nil {
		return fmt.Errorf("decode broker response: %w", err)
	}
	return nil
}

func containsStatus(statuses []int, status int) bool {
	for _, accepted := range statuses {
		if accepted == status {
			return true
		}
	}
	return false
}
