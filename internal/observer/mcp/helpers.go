// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package mcp

import (
	"fmt"
	"regexp"
	"time"

	"github.com/openchoreo/openchoreo/internal/observer/types"
)

var granularityPattern = regexp.MustCompile(`^[1-9][0-9]*[hdw]$`)

// strPtr returns a pointer to the string, or nil if the string is empty.
func strPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// parseRFC3339Time parses a time string in RFC3339 format.
func parseRFC3339Time(timeStr string) (time.Time, error) {
	t, err := time.Parse(time.RFC3339, timeStr)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid time format (expected RFC3339): %w", err)
	}
	return t, nil
}

// setDefaults applies default values for common query parameters.
func setDefaults(limit int, sortOrder string, logLevels []string) (int, string, []string) {
	if limit == 0 {
		limit = 100
	}
	if sortOrder == "" {
		sortOrder = "desc"
	}
	if logLevels == nil {
		logLevels = []string{}
	}
	return limit, sortOrder, logLevels
}

// validateComponentScope validates that the required scope fields are present.
func validateComponentScope(namespace, project, component string) error {
	if namespace == "" {
		return fmt.Errorf("namespace is required")
	}
	if component != "" && project == "" {
		return fmt.Errorf("project is required when component is provided")
	}
	return nil
}

// validateFinOpsScope validates the scope fields for FinOps queries, which
// additionally require an environment.
func validateFinOpsScope(namespace, environment, project, component string) error {
	if namespace == "" {
		return fmt.Errorf("namespace is required")
	}
	if environment == "" {
		return fmt.Errorf("environment is required")
	}
	if component != "" && project == "" {
		return fmt.Errorf("project is required when component is provided")
	}
	return nil
}

// validatePlatformScope validates the plane coordinates for a platform logs query. It mirrors the
// REST validator: the control plane is a singleton and "Other" is the absence of a plane, so
// neither takes a name; the namespace-scoped kinds additionally need the CR's namespace.
func validatePlatformScope(planeKind, planeName, planeNamespace string) error {
	req := &types.PlatformLogsQueryRequest{
		PlaneKind:      types.PlaneKind(planeKind),
		PlaneName:      planeName,
		PlaneNamespace: planeNamespace,
	}

	if planeKind == "" {
		return fmt.Errorf("plane_kind is required")
	}
	if _, ok := validPlaneKinds[req.PlaneKind]; !ok {
		return fmt.Errorf("plane_kind %q is not a valid plane kind", planeKind)
	}

	if !req.IsPlaneNameRequired() {
		if planeName != "" {
			return fmt.Errorf("plane_name is not applicable for plane_kind %s", planeKind)
		}
		return nil
	}
	if planeName == "" {
		return fmt.Errorf("plane_name is required for plane_kind %s", planeKind)
	}
	if req.IsPlaneNamespaced() && planeNamespace == "" {
		return fmt.Errorf("plane_namespace is required for plane_kind %s", planeKind)
	}
	return nil
}

// validPlaneKinds is the set accepted by the platform logs tool, kept in one place so the tool
// schema's enum and this validator cannot drift apart.
var validPlaneKinds = map[types.PlaneKind]struct{}{
	types.PlaneKindControlPlane:              {},
	types.PlaneKindDataPlane:                 {},
	types.PlaneKindClusterDataPlane:          {},
	types.PlaneKindWorkflowPlane:             {},
	types.PlaneKindClusterWorkflowPlane:      {},
	types.PlaneKindObservabilityPlane:        {},
	types.PlaneKindClusterObservabilityPlane: {},
	types.PlaneKindOther:                     {},
}

// planeKindNames returns the accepted plane kinds as strings, for the tool schema enum.
func planeKindNames() []string {
	return []string{
		string(types.PlaneKindControlPlane),
		string(types.PlaneKindDataPlane),
		string(types.PlaneKindClusterDataPlane),
		string(types.PlaneKindWorkflowPlane),
		string(types.PlaneKindClusterWorkflowPlane),
		string(types.PlaneKindObservabilityPlane),
		string(types.PlaneKindClusterObservabilityPlane),
		string(types.PlaneKindOther),
	}
}

func validateGranularity(granularity string) error {
	if granularity != "" && !granularityPattern.MatchString(granularity) {
		return fmt.Errorf("granularity must match <count><unit> notation (e.g. 1h, 2d, 3w)")
	}
	return nil
}
