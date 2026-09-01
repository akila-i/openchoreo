// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"context"
	"testing"

	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/config"
)

func TestGetPlatformObservability(t *testing.T) {
	tests := []struct {
		name        string
		ref         config.ObservabilityPlaneRefConfig
		wantEnabled bool
		wantKind    gen.ObservabilityPlaneRefKind
		wantName    string
	}{
		{
			name:        "unconfigured when no name is supplied",
			ref:         config.ObservabilityPlaneRefConfig{Kind: "ClusterObservabilityPlane"},
			wantEnabled: false,
		},
		{
			name:        "cluster-scoped reference is returned",
			ref:         config.ObservabilityPlaneRefConfig{Kind: "ClusterObservabilityPlane", Name: "default"},
			wantEnabled: true,
			wantKind:    gen.ObservabilityPlaneRefKindClusterObservabilityPlane,
			wantName:    "default",
		},
		{
			name:        "namespace-scoped reference is returned",
			ref:         config.ObservabilityPlaneRefConfig{Kind: "ObservabilityPlane", Name: "eu-obs"},
			wantEnabled: true,
			wantKind:    gen.ObservabilityPlaneRefKindObservabilityPlane,
			wantName:    "eu-obs",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := &Handler{
				Config: &config.Config{
					PlatformObservability: config.PlatformObservabilityConfig{
						ObservabilityPlaneRef: tt.ref,
					},
				},
			}

			resp, err := h.GetPlatformObservability(context.Background(),
				gen.GetPlatformObservabilityRequestObject{})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			got, ok := resp.(gen.GetPlatformObservability200JSONResponse)
			if !ok {
				t.Fatalf("unexpected response type: %T", resp)
			}

			if got.PlaneKind != gen.ControlPlane {
				t.Errorf("PlaneKind: got %q, want %q", got.PlaneKind, gen.ControlPlane)
			}
			if got.Enabled != tt.wantEnabled {
				t.Errorf("Enabled: got %v, want %v", got.Enabled, tt.wantEnabled)
			}

			if !tt.wantEnabled {
				if got.ObservabilityPlaneRef != nil {
					t.Errorf("ObservabilityPlaneRef: got %+v, want nil when unconfigured", got.ObservabilityPlaneRef)
				}
				return
			}

			if got.ObservabilityPlaneRef == nil {
				t.Fatal("ObservabilityPlaneRef: got nil, want a reference")
			}
			if got.ObservabilityPlaneRef.Kind != tt.wantKind {
				t.Errorf("Kind: got %q, want %q", got.ObservabilityPlaneRef.Kind, tt.wantKind)
			}
			if got.ObservabilityPlaneRef.Name != tt.wantName {
				t.Errorf("Name: got %q, want %q", got.ObservabilityPlaneRef.Name, tt.wantName)
			}
		})
	}
}
