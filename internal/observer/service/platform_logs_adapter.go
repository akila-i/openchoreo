// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/openchoreo/openchoreo/internal/observer/api/logsadapterclientgen"
	"github.com/openchoreo/openchoreo/internal/observer/types"
)

// PlatformLogsAdapterClient forwards platform log queries to the logs adapter over the generated
// client. It is separate from LogsAdapter because the platform endpoint is a distinct path on the
// adapter contract rather than another variant of the workload search scope.
type PlatformLogsAdapterClient struct {
	client *logsadapterclientgen.ClientWithResponses
	logger *slog.Logger
}

var _ PlatformLogsAdapter = (*PlatformLogsAdapterClient)(nil)

// NewPlatformLogsAdapterClient creates a client for the logs adapter's platform logs endpoint.
func NewPlatformLogsAdapterClient(
	baseURL string,
	timeout time.Duration,
	logger *slog.Logger,
) (*PlatformLogsAdapterClient, error) {
	if timeout == 0 {
		timeout = 30 * time.Second
	}
	client, err := logsadapterclientgen.NewClientWithResponses(
		baseURL,
		logsadapterclientgen.WithHTTPClient(&http.Client{Timeout: timeout}),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create logs adapter client: %w", err)
	}
	return &PlatformLogsAdapterClient{client: client, logger: logger}, nil
}

// GetPlatformLogs forwards the resolved query to the adapter.
func (a *PlatformLogsAdapterClient) GetPlatformLogs(
	ctx context.Context,
	scope *PlatformLogScope,
	req *types.PlatformLogsQueryRequest,
) (*types.PlatformLogsResponse, error) {
	startTime, err := time.Parse(time.RFC3339, req.StartTime)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid startTime: %w", ErrPlatformLogsRetrieval, err)
	}
	endTime, err := time.Parse(time.RFC3339, req.EndTime)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid endTime: %w", ErrPlatformLogsRetrieval, err)
	}

	body := logsadapterclientgen.PlatformLogsQueryRequest{
		StartTime: startTime,
		EndTime:   endTime,
		Scope:     buildAdapterScope(scope, req),
	}
	if req.Limit > 0 {
		limit := req.Limit
		body.Limit = &limit
	}
	if req.SortOrder != "" {
		sortOrder := logsadapterclientgen.PlatformLogsQueryRequestSortOrder(req.SortOrder)
		body.SortOrder = &sortOrder
	}
	if req.Labels != "" {
		labels := req.Labels
		body.Labels = &labels
	}
	if req.SearchPhrase != "" {
		searchPhrase := req.SearchPhrase
		body.SearchPhrase = &searchPhrase
	}

	resp, err := a.client.QueryPlatformLogsWithResponse(ctx, body)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrPlatformLogsRetrieval, err)
	}
	if resp.StatusCode() != http.StatusOK {
		return nil, fmt.Errorf("%w: logs adapter returned status %d", ErrPlatformLogsRetrieval, resp.StatusCode())
	}

	var result types.PlatformLogsResponse
	if err := json.Unmarshal(resp.Body, &result); err != nil {
		return nil, fmt.Errorf("%w: failed to decode adapter response: %w", ErrPlatformLogsRetrieval, err)
	}
	return &result, nil
}

// buildAdapterScope translates the resolved scope and the request's pod-level filters into the
// adapter's wire shape.
func buildAdapterScope(
	scope *PlatformLogScope,
	req *types.PlatformLogsQueryRequest,
) logsadapterclientgen.PlatformSearchScope {
	out := logsadapterclientgen.PlatformSearchScope{}

	if scope.Unattributed {
		unattributed := true
		out.Unattributed = &unattributed
	} else if scope.Plane != "" {
		plane := logsadapterclientgen.PlatformSearchScopePlane(scope.Plane)
		out.Plane = &plane
	}
	if scope.PlaneID != "" {
		planeID := scope.PlaneID
		out.PlaneId = &planeID
	}
	if req.ClusterInstance != "" {
		clusterInstance := req.ClusterInstance
		out.ClusterInstance = &clusterInstance
	}
	if len(req.Namespaces) > 0 {
		namespaces := req.Namespaces
		out.Namespaces = &namespaces
	}
	if len(req.PodNames) > 0 {
		podNames := req.PodNames
		out.PodNames = &podNames
	}
	if len(req.ContainerNames) > 0 {
		containerNames := req.ContainerNames
		out.ContainerNames = &containerNames
	}
	return out
}
