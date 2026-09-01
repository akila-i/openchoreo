// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	authzcore "github.com/openchoreo/openchoreo/internal/authz/core"
	coremocks "github.com/openchoreo/openchoreo/internal/authz/core/mocks"
	observerAuthz "github.com/openchoreo/openchoreo/internal/observer/authz"
	"github.com/openchoreo/openchoreo/internal/observer/service/mocks"
	"github.com/openchoreo/openchoreo/internal/observer/types"
)

func platformAuthzReq() *types.PlatformLogsQueryRequest {
	return &types.PlatformLogsQueryRequest{
		PlaneKind: types.PlaneKindDataPlane,
		PlaneName: "eu-dp",
		StartTime: "2026-08-14T16:30:00Z",
		EndTime:   "2026-08-14T17:30:00Z",
	}
}

func TestPlatformLogsAuthz_NilPDPSkipsCheck(t *testing.T) {
	inner := mocks.NewMockPlatformLogsQuerier(t)
	expected := &types.PlatformLogsResponse{}
	inner.EXPECT().QueryPlatformLogs(mock.Anything, mock.Anything).Return(expected, nil)

	svc := NewPlatformLogsServiceWithAuthz(inner, nil, testLogger())
	resp, err := svc.QueryPlatformLogs(context.Background(), platformAuthzReq())

	require.NoError(t, err)
	assert.Equal(t, expected, resp)
}

func TestPlatformLogsAuthz_Denied(t *testing.T) {
	inner := mocks.NewMockPlatformLogsQuerier(t)
	svc := NewPlatformLogsServiceWithAuthz(inner, mockPDPDeny(t), testLogger())

	_, err := svc.QueryPlatformLogs(authedCtx(), platformAuthzReq())

	require.ErrorIs(t, err, observerAuthz.ErrAuthzForbidden)
}

func TestPlatformLogsAuthz_Allowed(t *testing.T) {
	inner := mocks.NewMockPlatformLogsQuerier(t)
	expected := &types.PlatformLogsResponse{}
	inner.EXPECT().QueryPlatformLogs(mock.Anything, mock.Anything).Return(expected, nil)

	svc := NewPlatformLogsServiceWithAuthz(inner, mockPDPAllow(t), testLogger())
	resp, err := svc.QueryPlatformLogs(authedCtx(), platformAuthzReq())

	require.NoError(t, err)
	assert.Equal(t, expected, resp)
}

// Platform observability is one cluster-scoped permission. The PDP decides on the resource
// hierarchy, so anything placed there implies an enforcement the data cannot back: the plane labels
// are set by whoever can helm upgrade, not by the control plane. This test pins that the hierarchy
// stays empty and that the plane never leaks into it, because retrofitting per-plane authorization
// later is much harder than starting with it.
func TestPlatformLogsAuthz_UsesClusterScopeWithEmptyHierarchy(t *testing.T) {
	inner := mocks.NewMockPlatformLogsQuerier(t)
	inner.EXPECT().QueryPlatformLogs(mock.Anything, mock.Anything).
		Return(&types.PlatformLogsResponse{}, nil)

	var captured *authzcore.EvaluateRequest
	pdp := coremocks.NewMockPDP(t)
	pdp.EXPECT().Evaluate(mock.Anything, mock.Anything).
		Run(func(_ context.Context, req *authzcore.EvaluateRequest) {
			captured = req
		}).
		Return(&authzcore.Decision{Decision: true}, nil).Once()

	svc := NewPlatformLogsServiceWithAuthz(inner, pdp, testLogger())
	_, err := svc.QueryPlatformLogs(authedCtx(), platformAuthzReq())
	require.NoError(t, err)

	require.NotNil(t, captured)
	assert.Equal(t, string(observerAuthz.ActionViewPlatformLogs), captured.Action)
	assert.Equal(t, string(observerAuthz.ResourceTypePlatform), captured.Resource.Type)
	assert.Equal(t, authzcore.ResourceHierarchy{}, captured.Resource.Hierarchy,
		"platform logs must authorize at cluster scope; a non-empty hierarchy implies per-plane enforcement that is not real")
	assert.Empty(t, captured.Context.Resource.Environment)
}
