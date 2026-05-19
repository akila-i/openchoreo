// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package clustergateway

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gorilla/websocket"

	"github.com/openchoreo/openchoreo/internal/cluster-agent/messaging"
)

// handleWirelogs handles the wirelogs (Cilium Hubble flow) WebSocket endpoint.
// URL: /api/wirelogs/{planeType}/{planeID}/{crNamespace}/{crName}?component=...&environment=...&namespace=...
//
// Flow:
//  1. Upgrade the API-server-side connection to WebSocket.
//  2. Send a HTTPTunnelStreamInit{Target: "hubble"} to the data-plane agent
//     authorized for the CR; the agent will open a gRPC GetFlows stream against
//     hubble-relay and forward each flow as a HTTPTunnelStreamChunk.
//  3. Forward chunks one-way (agent → API-server). If the API server closes
//     its socket, send IsClose to the agent so it can cancel the gRPC stream.
func (s *Server) handleWirelogs(w http.ResponseWriter, r *http.Request) {
	requestID := getOrGenerateRequestID(r)
	logger := s.logger.With("requestId", requestID)

	// Parse URL: /api/wirelogs/{planeType}/{planeID}/{crNamespace}/{crName}
	path := strings.TrimPrefix(r.URL.Path, "/api/wirelogs/")
	parts := strings.SplitN(path, "/", 4)
	if len(parts) < 4 {
		http.Error(w, "invalid wirelogs URL: expected /api/wirelogs/{planeType}/{planeID}/{crNamespace}/{crName}", http.StatusBadRequest)
		return
	}
	planeType := parts[0]
	planeID := parts[1]
	crNamespace := parts[2]
	crName := parts[3]

	query := r.URL.Query()
	component := query.Get("component")
	environment := query.Get("environment")
	namespace := query.Get("namespace")
	if component == "" || environment == "" || namespace == "" {
		http.Error(w, "component, environment, and namespace query parameters are required", http.StatusBadRequest)
		return
	}

	planeIdentifier := fmt.Sprintf("%s/%s", planeType, planeID)
	if crNamespace == clusterCRNamespacePlaceholder {
		crNamespace = ""
	}
	crKey := fmt.Sprintf("%s/%s", crNamespace, crName)

	logger.Info("Wirelogs request received",
		"plane", planeIdentifier,
		"cr", crKey,
		"component", component,
		"environment", environment,
	)

	conn, err := s.connMgr.GetForCR(planeIdentifier, crKey)
	if err != nil {
		logger.Warn("No agent available for wirelogs", "error", err)
		http.Error(w, fmt.Sprintf("no agent available: %v", err), http.StatusServiceUnavailable)
		return
	}

	apiConn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		logger.Error("Failed to upgrade wirelogs to WebSocket", "error", err)
		return
	}
	defer apiConn.Close()

	session := &streamSession{
		requestID: requestID,
		fromAgent: make(chan *messaging.HTTPTunnelStreamChunk, 256),
		done:      make(chan struct{}),
	}

	s.registerStreamSession(requestID, session)
	defer s.unregisterStreamSession(requestID)

	agentQuery := url.Values{}
	agentQuery.Set("component", component)
	agentQuery.Set("environment", environment)
	agentQuery.Set("namespace", namespace)

	streamInit := &messaging.HTTPTunnelStreamInit{
		RequestID:    requestID,
		Target:       "hubble",
		Method:       "GET",
		Path:         "/wirelogs",
		Query:        agentQuery.Encode(),
		IsUpgrade:    true,
		UpgradeProto: "hubble/v1",
	}

	initData, err := json.Marshal(streamInit)
	if err != nil {
		logger.Error("Failed to marshal stream init", "error", err)
		return
	}

	if err := conn.SendRawMessage(initData); err != nil {
		logger.Error("Failed to send stream init to agent", "error", err)
		return
	}

	logger.Info("Wirelogs stream init sent to agent")

	// Wait for the agent's first chunk (sentinel) so we know the stream is live.
	select {
	case chunk := <-session.fromAgent:
		if chunk == nil {
			logger.Error("Stream session closed before wirelogs started")
			return
		}
		if chunk.IsClose {
			logger.Warn("Agent closed wirelogs stream immediately", "data", string(chunk.Data))
			return
		}
		// Forward any initial payload (usually empty sentinel byte from the agent).
		if len(chunk.Data) > 0 {
			if err := apiConn.WriteMessage(websocket.TextMessage, chunk.Data); err != nil {
				return
			}
		}
	case <-time.After(30 * time.Second):
		logger.Error("Timeout waiting for agent to start wirelogs stream")
		return
	case <-session.done:
		return
	}

	// API server → agent: only used to detect client close, which is forwarded
	// to the agent so it cancels the upstream gRPC stream against hubble-relay.
	go func() {
		defer session.close()
		for {
			if _, _, err := apiConn.ReadMessage(); err != nil {
				closeChunk, _ := json.Marshal(&messaging.HTTPTunnelStreamChunk{
					RequestID: requestID,
					IsClose:   true,
				})
				_ = conn.SendRawMessage(closeChunk)
				return
			}
		}
	}()

	// Agent → API server: forward each flow chunk as a WebSocket text frame.
	for {
		select {
		case chunk, ok := <-session.fromAgent:
			if !ok || chunk == nil {
				return
			}
			if chunk.IsClose {
				_ = apiConn.WriteMessage(websocket.CloseMessage,
					websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
				return
			}
			if len(chunk.Data) > 0 {
				if err := apiConn.WriteMessage(websocket.TextMessage, chunk.Data); err != nil {
					return
				}
			}
		case <-session.done:
			return
		}
	}
}
