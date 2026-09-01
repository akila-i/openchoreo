// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"strings"
	"testing"

	"github.com/openchoreo/openchoreo/internal/observer/types"
)

// testPlaneNamespace is the namespace of the plane CR in these fixtures, which is a different
// thing from the platform pod's namespace.
const testPlaneNamespace = "default"

func validPlatformReq() *types.PlatformLogsQueryRequest {
	return &types.PlatformLogsQueryRequest{
		PlaneKind: types.PlaneKindControlPlane,
		StartTime: "2026-08-14T16:30:00Z",
		EndTime:   "2026-08-14T17:30:00Z",
	}
}

func TestValidatePlatformLogsQueryRequest_PlaneScope(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*types.PlatformLogsQueryRequest)
		wantErr string
	}{
		{
			name:   "control plane needs no name",
			mutate: func(r *types.PlatformLogsQueryRequest) {},
		},
		{
			name:    "planeKind is required",
			mutate:  func(r *types.PlatformLogsQueryRequest) { r.PlaneKind = "" },
			wantErr: "planeKind is required",
		},
		{
			name:    "unknown planeKind is rejected",
			mutate:  func(r *types.PlatformLogsQueryRequest) { r.PlaneKind = "BuildPlane" },
			wantErr: "not a valid plane kind",
		},
		{
			// Silently ignoring it would return control-plane logs to a caller who asked for
			// something else.
			name: "planeName on the control plane is rejected",
			mutate: func(r *types.PlatformLogsQueryRequest) {
				r.PlaneName = "surprise"
			},
			wantErr: "planeName is not applicable",
		},
		{
			name: "Other takes no plane name",
			mutate: func(r *types.PlatformLogsQueryRequest) {
				r.PlaneKind = types.PlaneKindOther
				r.PlaneName = "cert-manager"
			},
			wantErr: "planeName is not applicable",
		},
		{
			name: "namespaced kind requires a name",
			mutate: func(r *types.PlatformLogsQueryRequest) {
				r.PlaneKind = types.PlaneKindDataPlane
				r.PlaneNamespace = testPlaneNamespace
			},
			wantErr: "planeName is required",
		},
		{
			name: "namespaced kind requires a plane namespace",
			mutate: func(r *types.PlatformLogsQueryRequest) {
				r.PlaneKind = types.PlaneKindDataPlane
				r.PlaneName = "eu-dp"
			},
			wantErr: "planeNamespace is required",
		},
		{
			name: "namespaced kind with both is accepted",
			mutate: func(r *types.PlatformLogsQueryRequest) {
				r.PlaneKind = types.PlaneKindWorkflowPlane
				r.PlaneName = "wp"
				r.PlaneNamespace = testPlaneNamespace
			},
		},
		{
			name: "cluster-scoped kind rejects a plane namespace",
			mutate: func(r *types.PlatformLogsQueryRequest) {
				r.PlaneKind = types.PlaneKindClusterDataPlane
				r.PlaneName = "shared-dp"
				r.PlaneNamespace = testPlaneNamespace
			},
			wantErr: "not applicable for the cluster-scoped",
		},
		{
			name: "cluster-scoped kind with a name only is accepted",
			mutate: func(r *types.PlatformLogsQueryRequest) {
				r.PlaneKind = types.PlaneKindClusterObservabilityPlane
				r.PlaneName = "shared-obs"
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := validPlatformReq()
			tt.mutate(req)
			err := ValidatePlatformLogsQueryRequest(req)

			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("expected an error containing %q, got nil", tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error %q does not contain %q", err.Error(), tt.wantErr)
			}
		})
	}
}

// Caps are enforced with a 400 rather than by truncating, which would silently return the wrong
// logs. Query strings have length limits a request body would not.
func TestValidatePlatformLogsQueryRequest_Caps(t *testing.T) {
	tooMany := make([]string, maxPlatformLogsFilterItems+1)
	for i := range tooMany {
		tooMany[i] = "ns"
	}
	atLimit := tooMany[:maxPlatformLogsFilterItems]

	tests := []struct {
		name    string
		mutate  func(*types.PlatformLogsQueryRequest)
		wantErr string
	}{
		{"namespace at the cap is accepted", func(r *types.PlatformLogsQueryRequest) { r.Namespaces = atLimit }, ""},
		{"namespace over the cap", func(r *types.PlatformLogsQueryRequest) { r.Namespaces = tooMany }, "namespace accepts at most"},
		{"podName over the cap", func(r *types.PlatformLogsQueryRequest) { r.PodNames = tooMany }, "podName accepts at most"},
		{"containerName over the cap", func(r *types.PlatformLogsQueryRequest) { r.ContainerNames = tooMany }, "containerName accepts at most"},
		{
			"searchPhrase over 256 characters",
			func(r *types.PlatformLogsQueryRequest) { r.SearchPhrase = strings.Repeat("a", maxSearchPhraseLength+1) },
			"searchPhrase cannot exceed",
		},
		{
			"labels over 256 characters",
			func(r *types.PlatformLogsQueryRequest) { r.Labels = strings.Repeat("a", maxLabelSelectorLength+1) },
			"labels cannot exceed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := validPlatformReq()
			tt.mutate(req)
			err := ValidatePlatformLogsQueryRequest(req)

			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error: got %v, want one containing %q", err, tt.wantErr)
			}
		})
	}
}

func TestValidatePlatformLogsQueryRequest_TimeAndDefaults(t *testing.T) {
	t.Run("missing time range is rejected", func(t *testing.T) {
		req := validPlatformReq()
		req.StartTime = ""
		if err := ValidatePlatformLogsQueryRequest(req); err == nil {
			t.Error("expected an error for a missing startTime")
		}
	})

	t.Run("defaults are applied", func(t *testing.T) {
		req := validPlatformReq()
		if err := ValidatePlatformLogsQueryRequest(req); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if req.Limit != defaultLimit {
			t.Errorf("Limit: got %d, want %d", req.Limit, defaultLimit)
		}
		if req.SortOrder != defaultSortOrder {
			t.Errorf("SortOrder: got %q, want %q", req.SortOrder, defaultSortOrder)
		}
	})
}
