// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"context"
	"log/slog"

	authzcore "github.com/openchoreo/openchoreo/internal/authz/core"
	observerAuthz "github.com/openchoreo/openchoreo/internal/observer/authz"
	"github.com/openchoreo/openchoreo/internal/observer/types"
)

// platformLogsServiceWithAuthz wraps a PlatformLogsQuerier and adds authorization checks.
// Both the HTTP handlers and the MCP handler should use this via NewPlatformLogsServiceWithAuthz.
type platformLogsServiceWithAuthz struct {
	internal PlatformLogsQuerier
	pdp      authzcore.PDP
	logger   *slog.Logger
}

var _ PlatformLogsQuerier = (*platformLogsServiceWithAuthz)(nil)

// NewPlatformLogsServiceWithAuthz wraps the provided PlatformLogsQuerier with authorization checks.
func NewPlatformLogsServiceWithAuthz(
	s PlatformLogsQuerier,
	pdp authzcore.PDP,
	logger *slog.Logger,
) PlatformLogsQuerier {
	return &platformLogsServiceWithAuthz{internal: s, pdp: pdp, logger: logger}
}

func (s *platformLogsServiceWithAuthz) QueryPlatformLogs(
	ctx context.Context,
	req *types.PlatformLogsQueryRequest,
) (*types.PlatformLogsResponse, error) {
	// Platform observability is operator-scoped, so this is a single cluster-scoped permission with
	// an empty resource hierarchy. Deliberately not per-plane: the PDP decides on the hierarchy, so
	// putting the plane name there would imply an enforcement that the data cannot back - the plane
	// labels are set by whoever can helm upgrade, not by the control plane. The boundary that does
	// hold is the collector's namespace allowlist, which keeps workload namespaces out of the
	// platform index entirely.
	if err := observerAuthz.CheckAuthorization(
		ctx, s.logger, s.pdp,
		observerAuthz.ActionViewPlatformLogs,
		observerAuthz.ResourceTypePlatform, "", authzcore.ResourceHierarchy{},
		authzcore.Context{},
	); err != nil {
		return nil, err
	}
	return s.internal.QueryPlatformLogs(ctx, req)
}
