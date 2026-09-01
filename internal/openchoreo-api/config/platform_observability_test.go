// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"testing"

	coreconfig "github.com/openchoreo/openchoreo/internal/config"
)

func TestPlatformObservabilityConfig_Configured(t *testing.T) {
	tests := []struct {
		name string
		ref  ObservabilityPlaneRefConfig
		want bool
	}{
		{"empty name is unconfigured", ObservabilityPlaneRefConfig{Kind: "ClusterObservabilityPlane"}, false},
		{"kind alone is unconfigured", ObservabilityPlaneRefConfig{Kind: "ObservabilityPlane"}, false},
		{"name makes it configured", ObservabilityPlaneRefConfig{Kind: "ObservabilityPlane", Name: "eu-obs"}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := PlatformObservabilityConfig{ObservabilityPlaneRef: tt.ref}
			if got := c.Configured(); got != tt.want {
				t.Errorf("Configured(): got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPlatformObservabilityConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		ref     ObservabilityPlaneRefConfig
		wantErr bool
	}{
		{
			// Platform observability is opt-in, so an unconfigured default must not fail startup.
			name:    "unconfigured passes even with a nonsense kind",
			ref:     ObservabilityPlaneRefConfig{Kind: "Nonsense"},
			wantErr: false,
		},
		{
			name:    "configured with ObservabilityPlane passes",
			ref:     ObservabilityPlaneRefConfig{Kind: "ObservabilityPlane", Name: "eu-obs"},
			wantErr: false,
		},
		{
			name:    "configured with ClusterObservabilityPlane passes",
			ref:     ObservabilityPlaneRefConfig{Kind: "ClusterObservabilityPlane", Name: "default"},
			wantErr: false,
		},
		{
			name:    "configured with an unknown kind fails",
			ref:     ObservabilityPlaneRefConfig{Kind: "DataPlane", Name: "default"},
			wantErr: true,
		},
		{
			name:    "configured with an empty kind fails",
			ref:     ObservabilityPlaneRefConfig{Name: "default"},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := PlatformObservabilityConfig{ObservabilityPlaneRef: tt.ref}
			errs := c.Validate(coreconfig.NewPath("platform_observability"))
			if tt.wantErr && len(errs) == 0 {
				t.Error("expected a validation error, got none")
			}
			if !tt.wantErr && len(errs) > 0 {
				t.Errorf("unexpected validation errors: %v", errs)
			}
		})
	}
}

func TestPlatformObservabilityDefaults(t *testing.T) {
	d := PlatformObservabilityDefaults()
	if d.Configured() {
		t.Error("defaults must be unconfigured: platform observability is opt-in")
	}
	if errs := d.Validate(coreconfig.NewPath("platform_observability")); len(errs) > 0 {
		t.Errorf("defaults must validate cleanly, got: %v", errs)
	}
}
