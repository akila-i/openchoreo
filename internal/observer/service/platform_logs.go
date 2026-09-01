// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/openchoreo/openchoreo/internal/observer/types"
)

var (
	// ErrPlatformLogsResolvePlane indicates a failure while resolving a plane CR to its planeID.
	ErrPlatformLogsResolvePlane = errors.New("platform logs plane resolution failed")
	// ErrPlatformLogsRetrieval indicates a failure while retrieving logs from the logs adapter.
	ErrPlatformLogsRetrieval = errors.New("platform logs retrieval failed")
)

// PlaneIDResolver resolves a plane CR to the planeID stamped on its pods.
type PlaneIDResolver interface {
	GetPlaneID(ctx context.Context, kind types.PlaneKind, namespace, name string) (string, error)
}

// PlatformLogsAdapter queries platform logs from the configured logs backend.
type PlatformLogsAdapter interface {
	GetPlatformLogs(ctx context.Context, scope *PlatformLogScope, req *types.PlatformLogsQueryRequest) (*types.PlatformLogsResponse, error)
}

// PlatformLogScope is the resolved, physical scope of a platform log query: what the collector
// actually stamped on the records, as opposed to the CR names the caller supplied.
type PlatformLogScope struct {
	// Plane is the openchoreo.dev/plane label value, empty when Unattributed is true.
	Plane string
	// Unattributed selects records carrying no plane label.
	Unattributed bool
	// PlaneID is the openchoreo.dev/plane-id label value. Empty for the control plane, which is a
	// singleton, and for unattributed records.
	PlaneID string
}

// PlatformLogsService serves platform (system component) logs. It resolves the caller's plane CR
// to physical identity and delegates the query to the logs adapter.
type PlatformLogsService struct {
	adapter  PlatformLogsAdapter
	resolver PlaneIDResolver
	logger   *slog.Logger
}

var _ PlatformLogsQuerier = (*PlatformLogsService)(nil)

// NewPlatformLogsService creates a new PlatformLogsService.
func NewPlatformLogsService(
	adapter PlatformLogsAdapter,
	resolver PlaneIDResolver,
	logger *slog.Logger,
) *PlatformLogsService {
	return &PlatformLogsService{adapter: adapter, resolver: resolver, logger: logger}
}

// QueryPlatformLogs resolves the requested plane to its planeID and queries the adapter.
func (s *PlatformLogsService) QueryPlatformLogs(
	ctx context.Context,
	req *types.PlatformLogsQueryRequest,
) (*types.PlatformLogsResponse, error) {
	if req == nil {
		return nil, fmt.Errorf("request must not be nil")
	}

	scope, err := s.resolveScope(ctx, req)
	if err != nil {
		return nil, err
	}

	s.logger.Debug("Querying platform logs",
		"planeKind", req.PlaneKind,
		"plane", scope.Plane,
		"planeID", scope.PlaneID,
		"unattributed", scope.Unattributed,
		"clusterInstance", req.ClusterInstance,
	)

	result, err := s.adapter.GetPlatformLogs(ctx, scope, req)
	if err != nil {
		return nil, err
	}
	if result.Logs == nil {
		result.Logs = []types.PlatformLog{}
	}
	return result, nil
}

// resolveScope turns the request's CR-level scope into the physical identity carried on the log
// records. Several CRs may share one planeID, which is correct: they describe the same
// installation, so they have the same logs.
func (s *PlatformLogsService) resolveScope(
	ctx context.Context,
	req *types.PlatformLogsQueryRequest,
) (*PlatformLogScope, error) {
	if req.PlaneKind == types.PlaneKindOther {
		return &PlatformLogScope{Unattributed: true}, nil
	}

	scope := &PlatformLogScope{Plane: req.PlaneLabel()}

	// The control plane is a singleton with no CR, so the plane label alone identifies it.
	if req.PlaneKind == types.PlaneKindControlPlane {
		return scope, nil
	}

	planeID, err := s.resolver.GetPlaneID(ctx, req.PlaneKind, req.PlaneNamespace, req.PlaneName)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrPlatformLogsResolvePlane, err)
	}
	scope.PlaneID = planeID
	return scope, nil
}
