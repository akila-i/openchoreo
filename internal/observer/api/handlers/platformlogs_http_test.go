// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	observerAuthz "github.com/openchoreo/openchoreo/internal/observer/authz"
	"github.com/openchoreo/openchoreo/internal/observer/service"
	servicemocks "github.com/openchoreo/openchoreo/internal/observer/service/mocks"
	"github.com/openchoreo/openchoreo/internal/observer/types"
)

const validPlatformLogsQuery = "?planeKind=ControlPlane" +
	"&startTime=2026-08-14T16:30:00Z&endTime=2026-08-14T17:30:00Z"

func platformLogsRequest(query string) *http.Request {
	return httptest.NewRequest(http.MethodGet, "/api/v1alpha1/platform-logs"+query, nil)
}

func TestGetPlatformLogs_Success(t *testing.T) {
	t.Parallel()

	svc := servicemocks.NewMockPlatformLogsQuerier(t)
	svc.On("QueryPlatformLogs", mock.Anything, mock.Anything).Return(&types.PlatformLogsResponse{
		Logs:   []types.PlatformLog{{Timestamp: "2026-08-14T16:31:00Z", Log: "reconciled"}},
		Total:  1,
		TookMs: 5,
	}, nil)

	h := &Handler{baseHandler: baseHandler{logger: noopLogger()}, platformLogsService: svc}
	rr := httptest.NewRecorder()

	h.GetPlatformLogs(rr, platformLogsRequest(validPlatformLogsQuery))

	require.Equal(t, http.StatusOK, rr.Code)
	assert.Contains(t, rr.Body.String(), `"total":1`)
}

// Repeated query parameters are how multi-value filters arrive on a GET.
func TestGetPlatformLogs_ParsesRepeatedParams(t *testing.T) {
	t.Parallel()

	var got *types.PlatformLogsQueryRequest
	svc := servicemocks.NewMockPlatformLogsQuerier(t)
	svc.On("QueryPlatformLogs", mock.Anything, mock.Anything).
		Run(func(args mock.Arguments) {
			got = args.Get(1).(*types.PlatformLogsQueryRequest)
		}).
		Return(&types.PlatformLogsResponse{Logs: []types.PlatformLog{}}, nil)

	h := &Handler{baseHandler: baseHandler{logger: noopLogger()}, platformLogsService: svc}
	rr := httptest.NewRecorder()

	h.GetPlatformLogs(rr, platformLogsRequest(validPlatformLogsQuery+
		"&namespace=openchoreo-control-plane&namespace=cert-manager"+
		"&podName=controller-manager-abc&containerName=manager"+
		"&clusterInstance=cluster1&limit=250&sortOrder=asc"))

	require.Equal(t, http.StatusOK, rr.Code)
	require.NotNil(t, got)
	assert.Equal(t, []string{"openchoreo-control-plane", "cert-manager"}, got.Namespaces)
	assert.Equal(t, []string{"controller-manager-abc"}, got.PodNames)
	assert.Equal(t, []string{"manager"}, got.ContainerNames)
	assert.Equal(t, "cluster1", got.ClusterInstance)
	assert.Equal(t, 250, got.Limit)
	assert.Equal(t, "asc", got.SortOrder)
}

func TestGetPlatformLogs_BadRequests(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		query string
	}{
		{"missing planeKind", "?startTime=2026-08-14T16:30:00Z&endTime=2026-08-14T17:30:00Z"},
		{"missing time range", "?planeKind=ControlPlane"},
		{"non-integer limit", validPlatformLogsQuery + "&limit=lots"},
		{"named plane without a name", "?planeKind=DataPlane&startTime=2026-08-14T16:30:00Z&endTime=2026-08-14T17:30:00Z"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// The service must never be reached; NewMockPlatformLogsQuerier fails the test on any
			// unexpected call.
			h := &Handler{
				baseHandler:         baseHandler{logger: noopLogger()},
				platformLogsService: servicemocks.NewMockPlatformLogsQuerier(t),
			}
			rr := httptest.NewRecorder()

			h.GetPlatformLogs(rr, platformLogsRequest(tt.query))

			assert.Equal(t, http.StatusBadRequest, rr.Code)
		})
	}
}

func TestGetPlatformLogs_ErrorMapping(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		err          error
		wantStatus   int
		wantContains string
	}{
		{"forbidden", observerAuthz.ErrAuthzForbidden, http.StatusForbidden, ""},
		{"unauthorized", observerAuthz.ErrAuthzUnauthorized, http.StatusUnauthorized, ""},
		{
			"plane resolution failure",
			service.ErrPlatformLogsResolvePlane,
			http.StatusInternalServerError,
			types.ErrorCodeV1PlatformLogsResolverFailed,
		},
		{
			"adapter failure",
			service.ErrPlatformLogsRetrieval,
			http.StatusInternalServerError,
			types.ErrorCodeV1PlatformLogsRetrievalFailed,
		},
		{
			"scope auth failure",
			service.ErrScopeAuthFailed,
			http.StatusInternalServerError,
			types.ErrorCodeV1ScopeAuthFailed,
		},
		{
			"unclassified failure",
			errors.New("boom"),
			http.StatusInternalServerError,
			types.ErrorCodeV1PlatformLogsInternalGeneric,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := servicemocks.NewMockPlatformLogsQuerier(t)
			svc.On("QueryPlatformLogs", mock.Anything, mock.Anything).Return(nil, tt.err)

			h := &Handler{baseHandler: baseHandler{logger: noopLogger()}, platformLogsService: svc}
			rr := httptest.NewRecorder()

			h.GetPlatformLogs(rr, platformLogsRequest(validPlatformLogsQuery))

			assert.Equal(t, tt.wantStatus, rr.Code)
			if tt.wantContains != "" {
				assert.Contains(t, rr.Body.String(), tt.wantContains)
			}
		})
	}
}

func TestGetPlatformLogs_ServiceNotInitialized(t *testing.T) {
	t.Parallel()

	h := &Handler{baseHandler: baseHandler{logger: noopLogger()}}
	rr := httptest.NewRecorder()

	h.GetPlatformLogs(rr, platformLogsRequest(validPlatformLogsQuery))

	assert.Equal(t, http.StatusInternalServerError, rr.Code)
	assert.Contains(t, rr.Body.String(), types.ErrorCodeV1PlatformLogsServiceNotReady)
}
