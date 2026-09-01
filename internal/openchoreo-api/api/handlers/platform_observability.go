// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"context"

	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
)

// GetPlatformObservability returns where the control plane's own platform (system component) logs
// are published.
//
// Every other plane carries this on its CR as spec.platformObservabilityPlaneRef. The control plane
// has no CR of its own, so the reference is supplied as a Helm configuration and read back here.
func (h *Handler) GetPlatformObservability(
	_ context.Context,
	_ gen.GetPlatformObservabilityRequestObject,
) (gen.GetPlatformObservabilityResponseObject, error) {
	cfg := h.Config.PlatformObservability

	response := gen.GetPlatformObservability200JSONResponse{
		PlaneKind: gen.ControlPlane,
		Enabled:   cfg.Configured(),
	}

	if cfg.Configured() {
		response.ObservabilityPlaneRef = &gen.ObservabilityPlaneRef{
			Kind: gen.ObservabilityPlaneRefKind(cfg.ObservabilityPlaneRef.Kind),
			Name: cfg.ObservabilityPlaneRef.Name,
		}
	}

	return response, nil
}
