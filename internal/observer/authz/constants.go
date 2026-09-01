// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package authz

type Action string

const (
	ActionViewLogs        Action = "logs:view"
	ActionViewEvents      Action = "events:view"
	ActionViewTraces      Action = "traces:view"
	ActionViewMetrics     Action = "metrics:view"
	ActionViewAlerts      Action = "alerts:view"
	ActionViewIncidents   Action = "incidents:view"
	ActionUpdateIncidents Action = "incidents:update"
	ActionViewFinOps      Action = "finops:view"

	// ActionViewPlatformLogs guards logs of OpenChoreo's own components. Operator-scoped and
	// cluster-wide, unlike the workload log actions which are scoped to a project or component.
	ActionViewPlatformLogs Action = "platformlogs:view"
)

type ResourceType string

const (
	ResourceTypeUnknown     ResourceType = "unknown"
	ResourceTypeComponent   ResourceType = "component"
	ResourceTypeProject     ResourceType = "project"
	ResourceTypeNamespace   ResourceType = "namespace"
	ResourceTypeWorkflowRun ResourceType = "workflowRun"

	// ResourceTypePlatform is the resource type for cluster-wide platform observability, which has
	// no project or component to hang off.
	ResourceTypePlatform ResourceType = "platform"
)
