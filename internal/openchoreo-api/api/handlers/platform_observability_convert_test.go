// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"testing"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
)

// The plane handlers convert between the CRD types and the generated API models with a JSON
// round-trip, and openapi/openchoreo-api.yaml is hand-maintained. So a mismatch between the Go
// json tag and the spec property name silently drops the field rather than failing to compile.
// These tests are what catches that.

func TestPlatformObservabilityPlaneRef_SurvivesConversion_Namespaced(t *testing.T) {
	ref := &openchoreov1alpha1.ObservabilityPlaneRef{
		Kind: openchoreov1alpha1.ObservabilityPlaneRefKindObservabilityPlane,
		Name: "eu-obs",
	}

	t.Run("DataPlane", func(t *testing.T) {
		src := openchoreov1alpha1.DataPlane{
			Spec: openchoreov1alpha1.DataPlaneSpec{PlatformObservabilityPlaneRef: ref},
		}
		got, err := convert[openchoreov1alpha1.DataPlane, gen.DataPlane](src)
		if err != nil {
			t.Fatalf("convert to gen: %v", err)
		}
		assertNamespacedRef(t, got.Spec.PlatformObservabilityPlaneRef)

		back, err := convert[gen.DataPlane, openchoreov1alpha1.DataPlane](got)
		if err != nil {
			t.Fatalf("convert back to CR: %v", err)
		}
		if back.Spec.PlatformObservabilityPlaneRef == nil {
			t.Fatal("ref lost converting back to the CR type")
		}
	})

	t.Run("WorkflowPlane", func(t *testing.T) {
		src := openchoreov1alpha1.WorkflowPlane{
			Spec: openchoreov1alpha1.WorkflowPlaneSpec{PlatformObservabilityPlaneRef: ref},
		}
		got, err := convert[openchoreov1alpha1.WorkflowPlane, gen.WorkflowPlane](src)
		if err != nil {
			t.Fatalf("convert to gen: %v", err)
		}
		assertNamespacedRef(t, got.Spec.PlatformObservabilityPlaneRef)
	})

	t.Run("ObservabilityPlane", func(t *testing.T) {
		src := openchoreov1alpha1.ObservabilityPlane{
			Spec: openchoreov1alpha1.ObservabilityPlaneSpec{
				ObserverURL:                   "http://observer:8080",
				PlatformObservabilityPlaneRef: ref,
			},
		}
		got, err := convert[openchoreov1alpha1.ObservabilityPlane, gen.ObservabilityPlane](src)
		if err != nil {
			t.Fatalf("convert to gen: %v", err)
		}
		assertNamespacedRef(t, got.Spec.PlatformObservabilityPlaneRef)
	})
}

func TestPlatformObservabilityPlaneRef_SurvivesConversion_ClusterScoped(t *testing.T) {
	ref := &openchoreov1alpha1.ClusterObservabilityPlaneRef{
		Kind: openchoreov1alpha1.ClusterObservabilityPlaneRefKindClusterObservabilityPlane,
		Name: "shared-obs",
	}

	t.Run("ClusterDataPlane", func(t *testing.T) {
		src := openchoreov1alpha1.ClusterDataPlane{
			Spec: openchoreov1alpha1.ClusterDataPlaneSpec{
				PlaneID:                       "prod",
				PlatformObservabilityPlaneRef: ref,
			},
		}
		got, err := convert[openchoreov1alpha1.ClusterDataPlane, gen.ClusterDataPlane](src)
		if err != nil {
			t.Fatalf("convert to gen: %v", err)
		}
		assertClusterRef(t, got.Spec.PlatformObservabilityPlaneRef)
	})

	t.Run("ClusterWorkflowPlane", func(t *testing.T) {
		src := openchoreov1alpha1.ClusterWorkflowPlane{
			Spec: openchoreov1alpha1.ClusterWorkflowPlaneSpec{
				PlaneID:                       "prod",
				PlatformObservabilityPlaneRef: ref,
			},
		}
		got, err := convert[openchoreov1alpha1.ClusterWorkflowPlane, gen.ClusterWorkflowPlane](src)
		if err != nil {
			t.Fatalf("convert to gen: %v", err)
		}
		assertClusterRef(t, got.Spec.PlatformObservabilityPlaneRef)
	})

	t.Run("ClusterObservabilityPlane", func(t *testing.T) {
		src := openchoreov1alpha1.ClusterObservabilityPlane{
			Spec: openchoreov1alpha1.ClusterObservabilityPlaneSpec{
				PlaneID:                       "prod",
				ObserverURL:                   "http://observer:8080",
				PlatformObservabilityPlaneRef: ref,
			},
		}
		got, err := convert[openchoreov1alpha1.ClusterObservabilityPlane, gen.ClusterObservabilityPlane](src)
		if err != nil {
			t.Fatalf("convert to gen: %v", err)
		}
		assertClusterRef(t, got.Spec.PlatformObservabilityPlaneRef)
	})
}

func assertNamespacedRef(t *testing.T, got *gen.ObservabilityPlaneRef) {
	t.Helper()
	if got == nil {
		t.Fatal("platformObservabilityPlaneRef was dropped in conversion")
	}
	if got.Kind != gen.ObservabilityPlaneRefKindObservabilityPlane {
		t.Errorf("Kind: got %q, want %q", got.Kind, gen.ObservabilityPlaneRefKindObservabilityPlane)
	}
	if got.Name != "eu-obs" {
		t.Errorf("Name: got %q, want %q", got.Name, "eu-obs")
	}
}

func assertClusterRef(t *testing.T, got *gen.ClusterObservabilityPlaneRef) {
	t.Helper()
	if got == nil {
		t.Fatal("platformObservabilityPlaneRef was dropped in conversion")
	}
	if got.Name != "shared-obs" {
		t.Errorf("Name: got %q, want %q", got.Name, "shared-obs")
	}
}
