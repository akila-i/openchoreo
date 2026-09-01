// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
)

// ─────────────────────────────────────────────────────────────
// Platform observability plane resolution
// ─────────────────────────────────────────────────────────────

func obsRef(name string) *openchoreov1alpha1.ObservabilityPlaneRef {
	return &openchoreov1alpha1.ObservabilityPlaneRef{
		Kind: openchoreov1alpha1.ObservabilityPlaneRefKindObservabilityPlane, Name: name,
	}
}

func clusterObsRef(name string) *openchoreov1alpha1.ClusterObservabilityPlaneRef {
	return &openchoreov1alpha1.ClusterObservabilityPlaneRef{
		Kind: openchoreov1alpha1.ClusterObservabilityPlaneRefKindClusterObservabilityPlane, Name: name,
	}
}

// platformObsTestNS is the namespace every namespace-scoped fixture in this file lives in.
const platformObsTestNS = "test-ns"

func nsObsPlane(name string) client.Object {
	return &openchoreov1alpha1.ObservabilityPlane{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: platformObsTestNS},
	}
}

func clusterObsPlane(name string) client.Object {
	return &openchoreov1alpha1.ClusterObservabilityPlane{ObjectMeta: metav1.ObjectMeta{Name: name}}
}

func TestGetPlatformObservabilityPlaneFromRef(t *testing.T) {
	scheme := newScheme(t)
	ctx := context.Background()

	objects := []client.Object{
		nsObsPlane("platform-obs"),
		nsObsPlane("workload-obs"),
		nsObsPlane("default"),
	}

	tests := []struct {
		name        string
		platformRef *openchoreov1alpha1.ObservabilityPlaneRef
		obsRef      *openchoreov1alpha1.ObservabilityPlaneRef
		wantName    string
	}{
		{
			name:        "platform ref wins when set",
			platformRef: obsRef("platform-obs"),
			obsRef:      obsRef("workload-obs"),
			wantName:    "platform-obs",
		},
		{
			name:        "falls back to the workload observability ref",
			platformRef: nil,
			obsRef:      obsRef("workload-obs"),
			wantName:    "workload-obs",
		},
		{
			name:        "falls back to the default plane when neither is set",
			platformRef: nil,
			obsRef:      nil,
			wantName:    "default",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fc := newFakeClient(t, scheme, objects...)
			got, err := GetPlatformObservabilityPlaneFromRef(ctx, fc, "test-ns", tt.platformRef, tt.obsRef)
			require.NoError(t, err)
			require.NotNil(t, got)
			assert.Equal(t, tt.wantName, got.GetName())
		})
	}
}

func TestDataPlaneResult_GetPlatformObservabilityPlane(t *testing.T) {
	scheme := newScheme(t)
	ctx := context.Background()

	tests := []struct {
		name     string
		result   *DataPlaneResult
		objects  []client.Object
		wantName string
		wantErr  string
	}{
		{
			name: "DataPlane platform ref is used over the workload ref",
			result: &DataPlaneResult{DataPlane: &openchoreov1alpha1.DataPlane{
				ObjectMeta: metav1.ObjectMeta{Name: "my-dp", Namespace: "test-ns"},
				Spec: openchoreov1alpha1.DataPlaneSpec{
					ObservabilityPlaneRef:         obsRef("workload-obs"),
					PlatformObservabilityPlaneRef: obsRef("platform-obs"),
				},
			}},
			objects:  []client.Object{nsObsPlane("workload-obs"), nsObsPlane("platform-obs")},
			wantName: "platform-obs",
		},
		{
			name: "DataPlane with no platform ref falls back to the workload ref",
			result: &DataPlaneResult{DataPlane: &openchoreov1alpha1.DataPlane{
				ObjectMeta: metav1.ObjectMeta{Name: "my-dp", Namespace: "test-ns"},
				Spec: openchoreov1alpha1.DataPlaneSpec{
					ObservabilityPlaneRef: obsRef("workload-obs"),
				},
			}},
			objects:  []client.Object{nsObsPlane("workload-obs")},
			wantName: "workload-obs",
		},
		{
			name: "ClusterDataPlane platform ref resolves cluster-scoped",
			result: &DataPlaneResult{ClusterDataPlane: &openchoreov1alpha1.ClusterDataPlane{
				ObjectMeta: metav1.ObjectMeta{Name: "cluster-dp"},
				Spec: openchoreov1alpha1.ClusterDataPlaneSpec{
					ObservabilityPlaneRef:         clusterObsRef("workload-obs"),
					PlatformObservabilityPlaneRef: clusterObsRef("platform-obs"),
				},
			}},
			objects:  []client.Object{clusterObsPlane("workload-obs"), clusterObsPlane("platform-obs")},
			wantName: "platform-obs",
		},
		{
			name:    "empty result errors",
			result:  &DataPlaneResult{},
			wantErr: "no data plane set in result",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fc := newFakeClient(t, scheme, tt.objects...)
			got, err := tt.result.GetPlatformObservabilityPlane(ctx, fc)

			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, got)
			assert.Equal(t, tt.wantName, got.GetName())
		})
	}
}

func TestWorkflowPlaneResult_GetPlatformObservabilityPlane(t *testing.T) {
	scheme := newScheme(t)
	ctx := context.Background()

	result := &WorkflowPlaneResult{WorkflowPlane: &openchoreov1alpha1.WorkflowPlane{
		ObjectMeta: metav1.ObjectMeta{Name: "my-wp", Namespace: "test-ns"},
		Spec: openchoreov1alpha1.WorkflowPlaneSpec{
			ObservabilityPlaneRef:         obsRef("workload-obs"),
			PlatformObservabilityPlaneRef: obsRef("platform-obs"),
		},
	}}
	fc := newFakeClient(t, scheme, nsObsPlane("workload-obs"), nsObsPlane("platform-obs"))

	got, err := result.GetPlatformObservabilityPlane(ctx, fc)
	require.NoError(t, err)
	assert.Equal(t, "platform-obs", got.GetName())

	_, err = (&WorkflowPlaneResult{}).GetPlatformObservabilityPlane(ctx, fc)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no workflow plane set in result")
}

// An observability plane serves its own platform logs unless told otherwise, so the no-ref case
// must return the same plane rather than looking for a "default".
func TestObservabilityPlaneResult_GetPlatformObservabilityPlane(t *testing.T) {
	scheme := newScheme(t)
	ctx := context.Background()

	t.Run("namespaced plane with no ref returns itself", func(t *testing.T) {
		self := &openchoreov1alpha1.ObservabilityPlane{
			ObjectMeta: metav1.ObjectMeta{Name: "eu-obs", Namespace: "test-ns"},
		}
		result := &ObservabilityPlaneResult{ObservabilityPlane: self}
		// Deliberately no "default" plane in the cluster: falling back would fail here.
		fc := newFakeClient(t, scheme)

		got, err := result.GetPlatformObservabilityPlane(ctx, fc)
		require.NoError(t, err)
		assert.Equal(t, "eu-obs", got.GetName())
	})

	t.Run("cluster plane with no ref returns itself", func(t *testing.T) {
		result := &ObservabilityPlaneResult{ClusterObservabilityPlane: &openchoreov1alpha1.ClusterObservabilityPlane{
			ObjectMeta: metav1.ObjectMeta{Name: "shared-obs"},
		}}
		fc := newFakeClient(t, scheme)

		got, err := result.GetPlatformObservabilityPlane(ctx, fc)
		require.NoError(t, err)
		assert.Equal(t, "shared-obs", got.GetName())
	})

	t.Run("explicit ref redirects elsewhere", func(t *testing.T) {
		result := &ObservabilityPlaneResult{ObservabilityPlane: &openchoreov1alpha1.ObservabilityPlane{
			ObjectMeta: metav1.ObjectMeta{Name: "eu-obs", Namespace: "test-ns"},
			Spec: openchoreov1alpha1.ObservabilityPlaneSpec{
				PlatformObservabilityPlaneRef: obsRef("central-obs"),
			},
		}}
		fc := newFakeClient(t, scheme, nsObsPlane("central-obs"))

		got, err := result.GetPlatformObservabilityPlane(ctx, fc)
		require.NoError(t, err)
		assert.Equal(t, "central-obs", got.GetName())
	})

	t.Run("empty result errors", func(t *testing.T) {
		_, err := (&ObservabilityPlaneResult{}).GetPlatformObservabilityPlane(ctx, newFakeClient(t, scheme))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no observability plane set in result")
	})
}

// The ClusterDataPlane -> DataPlane projection must carry the platform ref, or a consumer reading
// the projected DataPlane silently loses it.
func TestDataPlaneResult_ToDataPlane_CarriesPlatformObsRef(t *testing.T) {
	result := &DataPlaneResult{ClusterDataPlane: &openchoreov1alpha1.ClusterDataPlane{
		ObjectMeta: metav1.ObjectMeta{Name: "cluster-dp"},
		Spec: openchoreov1alpha1.ClusterDataPlaneSpec{
			PlatformObservabilityPlaneRef: clusterObsRef("platform-obs"),
		},
	}}

	dp := result.ToDataPlane()
	require.NotNil(t, dp)
	require.NotNil(t, dp.Spec.PlatformObservabilityPlaneRef)
	assert.Equal(t, "platform-obs", dp.Spec.PlatformObservabilityPlaneRef.Name)
	assert.Equal(t, openchoreov1alpha1.ObservabilityPlaneRefKindClusterObservabilityPlane,
		dp.Spec.PlatformObservabilityPlaneRef.Kind)
}
