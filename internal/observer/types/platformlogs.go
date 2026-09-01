// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package types

// PlaneKind identifies which plane CR a platform log query is scoped to.
// These are the CR kinds, not the pod label values - see PlaneLabel for the mapping.
type PlaneKind string

const (
	PlaneKindControlPlane              PlaneKind = "ControlPlane"
	PlaneKindDataPlane                 PlaneKind = "DataPlane"
	PlaneKindClusterDataPlane          PlaneKind = "ClusterDataPlane"
	PlaneKindWorkflowPlane             PlaneKind = "WorkflowPlane"
	PlaneKindClusterWorkflowPlane      PlaneKind = "ClusterWorkflowPlane"
	PlaneKindObservabilityPlane        PlaneKind = "ObservabilityPlane"
	PlaneKindClusterObservabilityPlane PlaneKind = "ClusterObservabilityPlane"

	// PlaneKindOther selects records carrying no openchoreo.dev/plane label - components
	// OpenChoreo depends on but does not ship, such as cert-manager or external-secrets.
	PlaneKindOther PlaneKind = "Other"
)

// Plane label values. The openchoreo.dev/plane pod label collapses the cluster-scoped and
// namespace-scoped CR kinds into one physical kind, because an operator reading logs does not
// care which flavor of CR describes the plane.
const (
	PlaneLabelControlPlane       = "controlplane"
	PlaneLabelDataPlane          = "dataplane"
	PlaneLabelWorkflowPlane      = "workflowplane"
	PlaneLabelObservabilityPlane = "observabilityplane"
)

// PlatformLogsQueryRequest represents the query parameters of
// GET /api/v1alpha1/platform-logs.
type PlatformLogsQueryRequest struct {
	// PlaneKind selects the plane type. Required.
	PlaneKind PlaneKind `json:"planeKind"`

	// PlaneName is the plane CR's name. Required for every kind except ControlPlane and Other.
	PlaneName string `json:"planeName,omitempty"`

	// PlaneNamespace is the namespace the plane CR lives in, required for the namespace-scoped
	// kinds. This is not the namespace of the pods whose logs are returned - see Namespaces.
	PlaneNamespace string `json:"planeNamespace,omitempty"`

	// ClusterInstance narrows to one cluster. For PlaneKindOther it is the only scope available.
	ClusterInstance string `json:"clusterInstance,omitempty"`

	// Namespaces filters on the Kubernetes namespace of the platform pod.
	Namespaces     []string `json:"namespaces,omitempty"`
	PodNames       []string `json:"podNames,omitempty"`
	ContainerNames []string `json:"containerNames,omitempty"`

	// Time range for the query. Both required, absolute RFC3339 UTC.
	StartTime string `json:"startTime"`
	EndTime   string `json:"endTime"`

	// Optional filters.
	Labels       string `json:"labels,omitempty"`
	SearchPhrase string `json:"searchPhrase,omitempty"`

	// Paging and sorting.
	Limit     int    `json:"limit,omitempty"`
	SortOrder string `json:"sortOrder,omitempty"`
}

// IsPlaneNameRequired reports whether the request's plane kind identifies a CR that has to be
// named. The control plane is a singleton and Other is the absence of a plane, so neither does.
func (r *PlatformLogsQueryRequest) IsPlaneNameRequired() bool {
	switch r.PlaneKind {
	case PlaneKindControlPlane, PlaneKindOther:
		return false
	default:
		return true
	}
}

// IsPlaneNamespaced reports whether the request's plane kind is a namespace-scoped CR, which
// additionally needs PlaneNamespace to be resolvable.
func (r *PlatformLogsQueryRequest) IsPlaneNamespaced() bool {
	switch r.PlaneKind {
	case PlaneKindDataPlane, PlaneKindWorkflowPlane, PlaneKindObservabilityPlane:
		return true
	default:
		return false
	}
}

// PlaneLabel maps the request's CR kind to the value carried by the openchoreo.dev/plane pod
// label. It returns an empty string for PlaneKindOther, which matches the absence of the label.
func (r *PlatformLogsQueryRequest) PlaneLabel() string {
	switch r.PlaneKind {
	case PlaneKindControlPlane:
		return PlaneLabelControlPlane
	case PlaneKindDataPlane, PlaneKindClusterDataPlane:
		return PlaneLabelDataPlane
	case PlaneKindWorkflowPlane, PlaneKindClusterWorkflowPlane:
		return PlaneLabelWorkflowPlane
	case PlaneKindObservabilityPlane, PlaneKindClusterObservabilityPlane:
		return PlaneLabelObservabilityPlane
	default:
		return ""
	}
}

// PlatformLog represents a single platform log entry in the response.
type PlatformLog struct {
	Timestamp       string `json:"timestamp"`
	Log             string `json:"log"`
	Level           string `json:"level,omitempty"`
	PlaneKind       string `json:"planeKind,omitempty"`
	PlaneID         string `json:"planeId,omitempty"`
	ClusterInstance string `json:"clusterInstance,omitempty"`
	NamespaceName   string `json:"namespaceName,omitempty"`
	PodName         string `json:"podName,omitempty"`
	ContainerName   string `json:"containerName,omitempty"`
}

// PlatformLogsResponse represents the response for GET /api/v1alpha1/platform-logs.
type PlatformLogsResponse struct {
	Logs   []PlatformLog `json:"logs"`
	Total  int64         `json:"total"`
	TookMs int64         `json:"tookMs"`
}
