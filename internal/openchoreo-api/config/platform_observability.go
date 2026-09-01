// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package config

import (
	coreconfig "github.com/openchoreo/openchoreo/internal/config"
)

// PlatformObservabilityConfig points at the observability plane that serves the control plane's own
// platform (system component) logs.
//
// Every other plane carries this on its CR as spec.platformObservabilityPlaneRef. The control plane
// has no CR of its own, so the reference is supplied here and served over
// GET /api/v1/platform-observability.
type PlatformObservabilityConfig struct {
	// ObservabilityPlaneRef references the observability plane serving control-plane platform logs.
	// Leaving Name empty means no destination is configured.
	ObservabilityPlaneRef ObservabilityPlaneRefConfig `koanf:"observability_plane_ref"`
}

// ObservabilityPlaneRefConfig mirrors api/v1alpha1.ObservabilityPlaneRef.
type ObservabilityPlaneRefConfig struct {
	// Kind is either ObservabilityPlane or ClusterObservabilityPlane.
	Kind string `koanf:"kind"`
	// Name is the name of the referenced resource.
	Name string `koanf:"name"`
}

// Configured reports whether a usable reference was supplied.
func (c *PlatformObservabilityConfig) Configured() bool {
	return c.ObservabilityPlaneRef.Name != ""
}

// PlatformObservabilityDefaults returns the default platform observability configuration.
// Platform observability is opt-in, so the default is unconfigured.
func PlatformObservabilityDefaults() PlatformObservabilityConfig {
	return PlatformObservabilityConfig{
		ObservabilityPlaneRef: ObservabilityPlaneRefConfig{
			Kind: "ClusterObservabilityPlane",
		},
	}
}

// Validate validates the platform observability configuration.
func (c *PlatformObservabilityConfig) Validate(path *coreconfig.Path) coreconfig.ValidationErrors {
	var errs coreconfig.ValidationErrors
	if !c.Configured() {
		return errs
	}
	refPath := path.Child("observability_plane_ref")
	switch c.ObservabilityPlaneRef.Kind {
	case "ObservabilityPlane", "ClusterObservabilityPlane":
	default:
		errs = append(errs, coreconfig.Invalid(refPath.Child("kind"),
			"must be ObservabilityPlane or ClusterObservabilityPlane"))
	}
	return errs
}
