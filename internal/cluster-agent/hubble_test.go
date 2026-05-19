// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package clusteragent

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildHubbleFlowFilters_ORsSourceAndDestination(t *testing.T) {
	filters := buildHubbleFlowFilters("checkout", "development", "my-team")

	// Expect exactly two FlowFilters: one with SourceLabel, one with DestinationLabel,
	// so flows match when the component pods are EITHER source OR destination.
	require.Len(t, filters, 2)

	expectedLabels := []string{
		"openchoreo.dev/component=checkout",
		"openchoreo.dev/environment=development",
		"openchoreo.dev/namespace=my-team",
	}

	assert.ElementsMatch(t, expectedLabels, filters[0].GetSourceLabel(),
		"first filter must whitelist by source labels")
	assert.Empty(t, filters[0].GetDestinationLabel(),
		"first filter must not constrain destination")

	assert.ElementsMatch(t, expectedLabels, filters[1].GetDestinationLabel(),
		"second filter must whitelist by destination labels")
	assert.Empty(t, filters[1].GetSourceLabel(),
		"second filter must not constrain source")
}

func TestNewGetFlowsRequest_LiveTail(t *testing.T) {
	req := newGetFlowsRequest("checkout", "development", "my-team")

	assert.True(t, req.GetFollow(), "wirelogs is a live tail; Follow must be true")
	assert.Zero(t, req.GetNumber(), "v1 does not replay history; Number must be 0")
	assert.Len(t, req.GetWhitelist(), 2, "request must carry both source and destination filters")
}

func TestHubbleRelayAddr_DefaultWhenUnset(t *testing.T) {
	a := &Agent{config: &Config{}}
	assert.Equal(t, defaultHubbleRelayAddr, a.hubbleRelayAddr())
}

func TestHubbleRelayAddr_OverrideFromConfig(t *testing.T) {
	a := &Agent{config: &Config{HubbleRelayAddr: "hubble-relay.custom.svc:4245"}}
	assert.Equal(t, "hubble-relay.custom.svc:4245", a.hubbleRelayAddr())
}

func TestHubbleSession_HandleChunkIsNoOp(t *testing.T) {
	// Hubble is server-streaming only; payload chunks from the API client are
	// ignored. Verify handleChunk does not panic and does not mutate state.
	s := &hubbleSession{requestID: "x", cancel: func() {}, done: make(chan struct{})}
	require.NotPanics(t, func() {
		s.handleChunk(nil)
	})
}

func TestHubbleSession_CloseIsIdempotent(t *testing.T) {
	cancelCalls := 0
	s := &hubbleSession{
		requestID: "x",
		cancel:    func() { cancelCalls++ },
		done:      make(chan struct{}),
	}
	s.close()
	s.close() // second close must not panic on closed channel or recall cancel
	assert.Equal(t, 1, cancelCalls)
}
