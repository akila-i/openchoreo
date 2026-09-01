// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"context"
	"errors"
	"log/slog"
	"testing"

	"github.com/openchoreo/openchoreo/internal/observer/types"
)

type fakePlaneIDResolver struct {
	planeID string
	err     error

	gotKind      types.PlaneKind
	gotNamespace string
	gotName      string
	calls        int
}

func (f *fakePlaneIDResolver) GetPlaneID(_ context.Context, kind types.PlaneKind, namespace, name string) (string, error) {
	f.calls++
	f.gotKind, f.gotNamespace, f.gotName = kind, namespace, name
	return f.planeID, f.err
}

type fakePlatformLogsAdapter struct {
	resp *types.PlatformLogsResponse
	err  error

	gotScope *PlatformLogScope
	gotReq   *types.PlatformLogsQueryRequest
}

func (f *fakePlatformLogsAdapter) GetPlatformLogs(
	_ context.Context, scope *PlatformLogScope, req *types.PlatformLogsQueryRequest,
) (*types.PlatformLogsResponse, error) {
	f.gotScope, f.gotReq = scope, req
	if f.err != nil {
		return nil, f.err
	}
	if f.resp != nil {
		return f.resp, nil
	}
	return &types.PlatformLogsResponse{}, nil
}

func newTestPlatformLogsService(
	adapter *fakePlatformLogsAdapter, resolver *fakePlaneIDResolver,
) *PlatformLogsService {
	return NewPlatformLogsService(adapter, resolver, slog.Default())
}

func baseReq(kind types.PlaneKind) *types.PlatformLogsQueryRequest {
	return &types.PlatformLogsQueryRequest{
		PlaneKind: kind,
		StartTime: "2026-08-14T16:30:00Z",
		EndTime:   "2026-08-14T17:30:00Z",
	}
}

// The seven CR kinds collapse to four label values, because the pod label does not distinguish
// the cluster-scoped from the namespace-scoped flavor of a plane.
func TestPlatformLogsService_CollapsesPlaneKindToLabel(t *testing.T) {
	tests := []struct {
		kind      types.PlaneKind
		wantPlane string
	}{
		{types.PlaneKindControlPlane, types.PlaneLabelControlPlane},
		{types.PlaneKindDataPlane, types.PlaneLabelDataPlane},
		{types.PlaneKindClusterDataPlane, types.PlaneLabelDataPlane},
		{types.PlaneKindWorkflowPlane, types.PlaneLabelWorkflowPlane},
		{types.PlaneKindClusterWorkflowPlane, types.PlaneLabelWorkflowPlane},
		{types.PlaneKindObservabilityPlane, types.PlaneLabelObservabilityPlane},
		{types.PlaneKindClusterObservabilityPlane, types.PlaneLabelObservabilityPlane},
	}

	for _, tt := range tests {
		t.Run(string(tt.kind), func(t *testing.T) {
			adapter := &fakePlatformLogsAdapter{}
			resolver := &fakePlaneIDResolver{planeID: "prod"}
			svc := newTestPlatformLogsService(adapter, resolver)

			req := baseReq(tt.kind)
			req.PlaneName = "some-plane"
			req.PlaneNamespace = "default"

			if _, err := svc.QueryPlatformLogs(context.Background(), req); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if adapter.gotScope.Plane != tt.wantPlane {
				t.Errorf("Plane: got %q, want %q", adapter.gotScope.Plane, tt.wantPlane)
			}
			if adapter.gotScope.Unattributed {
				t.Error("Unattributed must be false for an attributed plane kind")
			}
		})
	}
}

// The control plane is a singleton with no CR, so it must not trigger a resolver call.
func TestPlatformLogsService_ControlPlaneSkipsResolution(t *testing.T) {
	adapter := &fakePlatformLogsAdapter{}
	resolver := &fakePlaneIDResolver{planeID: "should-not-be-used"}
	svc := newTestPlatformLogsService(adapter, resolver)

	if _, err := svc.QueryPlatformLogs(context.Background(), baseReq(types.PlaneKindControlPlane)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resolver.calls != 0 {
		t.Errorf("resolver called %d times, want 0 for the control plane", resolver.calls)
	}
	if adapter.gotScope.PlaneID != "" {
		t.Errorf("PlaneID: got %q, want empty for the control plane", adapter.gotScope.PlaneID)
	}
}

// "Other" is the absence of a plane label, not a plane of its own.
func TestPlatformLogsService_OtherIsUnattributed(t *testing.T) {
	adapter := &fakePlatformLogsAdapter{}
	resolver := &fakePlaneIDResolver{}
	svc := newTestPlatformLogsService(adapter, resolver)

	req := baseReq(types.PlaneKindOther)
	req.ClusterInstance = "cluster1"

	if _, err := svc.QueryPlatformLogs(context.Background(), req); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !adapter.gotScope.Unattributed {
		t.Error("Unattributed: got false, want true for planeKind Other")
	}
	if adapter.gotScope.Plane != "" {
		t.Errorf("Plane: got %q, want empty for planeKind Other", adapter.gotScope.Plane)
	}
	if resolver.calls != 0 {
		t.Errorf("resolver called %d times, want 0 for planeKind Other", resolver.calls)
	}
}

func TestPlatformLogsService_ResolvesNamedPlane(t *testing.T) {
	adapter := &fakePlatformLogsAdapter{}
	resolver := &fakePlaneIDResolver{planeID: "dp-eu-1"}
	svc := newTestPlatformLogsService(adapter, resolver)

	req := baseReq(types.PlaneKindDataPlane)
	req.PlaneName = "eu-dp"
	req.PlaneNamespace = "tenant-a"

	if _, err := svc.QueryPlatformLogs(context.Background(), req); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resolver.gotKind != types.PlaneKindDataPlane {
		t.Errorf("resolver kind: got %q, want %q", resolver.gotKind, types.PlaneKindDataPlane)
	}
	// planeNamespace is the CR's namespace and must not be confused with the pod namespace filter.
	if resolver.gotNamespace != "tenant-a" || resolver.gotName != "eu-dp" {
		t.Errorf("resolver scope: got (%q, %q), want (tenant-a, eu-dp)", resolver.gotNamespace, resolver.gotName)
	}
	if adapter.gotScope.PlaneID != "dp-eu-1" {
		t.Errorf("PlaneID: got %q, want dp-eu-1", adapter.gotScope.PlaneID)
	}
}

func TestPlatformLogsService_ResolverFailureIsWrapped(t *testing.T) {
	adapter := &fakePlatformLogsAdapter{}
	resolver := &fakePlaneIDResolver{err: errors.New("boom")}
	svc := newTestPlatformLogsService(adapter, resolver)

	req := baseReq(types.PlaneKindClusterDataPlane)
	req.PlaneName = "shared-dp"

	_, err := svc.QueryPlatformLogs(context.Background(), req)
	if !errors.Is(err, ErrPlatformLogsResolvePlane) {
		t.Errorf("error: got %v, want it to wrap ErrPlatformLogsResolvePlane", err)
	}
}

// A nil Logs slice would serialize as JSON null, which clients have to special-case.
func TestPlatformLogsService_EmptyResultIsAnEmptySlice(t *testing.T) {
	adapter := &fakePlatformLogsAdapter{resp: &types.PlatformLogsResponse{Logs: nil}}
	svc := newTestPlatformLogsService(adapter, &fakePlaneIDResolver{})

	got, err := svc.QueryPlatformLogs(context.Background(), baseReq(types.PlaneKindControlPlane))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Logs == nil {
		t.Error("Logs: got nil, want an empty slice")
	}
}
