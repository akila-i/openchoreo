// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"

	"github.com/gorilla/websocket"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	authz "github.com/openchoreo/openchoreo/internal/authz/core"
	gatewayClient "github.com/openchoreo/openchoreo/internal/clients/gateway"
	"github.com/openchoreo/openchoreo/internal/controller"
	svcpkg "github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
)

// WirelogsHandler handles WebSocket wirelogs requests for component pods.
// It streams Cilium Hubble flow events for the component via the cluster-gateway
// tunnel to the data plane cluster-agent.
type WirelogsHandler struct {
	k8sClient      client.Client
	gatewayClient  *gatewayClient.Client
	gatewayURL     string
	gatewayTLSConf *tls.Config
	authzChecker   *svcpkg.AuthzChecker
	logger         *slog.Logger
}

// NewWirelogsHandler creates a new wirelogs handler.
func NewWirelogsHandler(k8sClient client.Client, gwClient *gatewayClient.Client, gatewayURL string, gwTLSConf *tls.Config, authzChecker *svcpkg.AuthzChecker, logger *slog.Logger) *WirelogsHandler {
	return &WirelogsHandler{
		k8sClient:      k8sClient,
		gatewayClient:  gwClient,
		gatewayURL:     gatewayURL,
		gatewayTLSConf: gwTLSConf,
		authzChecker:   authzChecker,
		logger:         logger.With("component", "wirelogs-handler"),
	}
}

var wirelogsUpgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

// ServeHTTP handles the wirelogs WebSocket upgrade and streams Hubble flow events.
// URL: /wirelogs/namespaces/{namespace}/components/{component}?environment=...&project=...
func (h *WirelogsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Parse URL path: /wirelogs/namespaces/{namespace}/components/{component}
	path := strings.TrimPrefix(r.URL.Path, "/wirelogs/namespaces/")
	parts := strings.SplitN(path, "/components/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		http.Error(w, "invalid wirelogs URL: expected /wirelogs/namespaces/{ns}/components/{name}", http.StatusBadRequest)
		return
	}
	namespace := parts[0]
	componentName := parts[1]

	query := r.URL.Query()
	project := query.Get("project")
	envName := query.Get("environment")

	ctx := r.Context()
	logger := h.logger.With("namespace", namespace, "component", componentName)

	// Authorize: caller must have logs:view on the component.
	if h.authzChecker == nil {
		logger.Error("Authorization checker not configured")
		http.Error(w, "authorization not configured", http.StatusInternalServerError)
		return
	}
	if err := h.authzChecker.Check(ctx, svcpkg.CheckRequest{
		Action:       authz.ActionViewLogs,
		ResourceType: "component",
		ResourceID:   componentName,
		Hierarchy: authz.ResourceHierarchy{
			Namespace: namespace,
			Project:   project,
		},
	}); err != nil {
		if errors.Is(err, svcpkg.ErrForbidden) {
			http.Error(w, "you do not have permission to view wirelogs for this component", http.StatusForbidden)
			return
		}
		logger.Error("Authorization check failed", "error", err)
		http.Error(w, "authorization check failed", http.StatusInternalServerError)
		return
	}

	// Resolve component → environment → data plane.
	plane, err := h.resolvePlane(ctx, namespace, componentName, project, &envName)
	if err != nil {
		logger.Error("Failed to resolve data plane for wirelogs", "error", err)
		http.Error(w, fmt.Sprintf("failed to resolve data plane: %v", err), http.StatusBadRequest)
		return
	}

	logger = logger.With("environment", envName,
		"planeType", plane.planeType, "planeID", plane.planeID)

	// Upgrade client connection to WebSocket.
	clientConn, err := wirelogsUpgrader.Upgrade(w, r, nil)
	if err != nil {
		logger.Error("Failed to upgrade to WebSocket", "error", err)
		return
	}
	defer clientConn.Close()

	gwURL, err := h.buildGatewayWirelogsURL(plane, componentName, envName, namespace)
	if err != nil {
		logger.Error("Failed to build gateway wirelogs URL", "error", err)
		writeWSError(clientConn, fmt.Sprintf("internal error: %v", err))
		return
	}

	gwDialer := websocket.Dialer{
		TLSClientConfig: h.gatewayTLSConf,
	}
	gwConn, _, err := gwDialer.DialContext(ctx, gwURL, nil)
	if err != nil {
		logger.Error("Failed to connect to gateway wirelogs endpoint", "error", err)
		writeWSError(clientConn, fmt.Sprintf("failed to connect to data plane: %v", err))
		return
	}
	defer gwConn.Close()

	logger.Info("Wirelogs session established")

	done := make(chan struct{}, 2)

	// client → gateway: client never sends payload; only detect close so the
	// gateway can forward IsClose to the agent (canceling the upstream gRPC stream).
	go func() {
		defer func() { done <- struct{}{} }()
		for {
			if _, _, err := clientConn.ReadMessage(); err != nil {
				return
			}
		}
	}()

	// gateway → client: each text frame is one Hubble flow JSON (NDJSON-on-WS).
	go func() {
		defer func() { done <- struct{}{} }()
		for {
			msgType, msg, err := gwConn.ReadMessage()
			if err != nil {
				closeCode := websocket.CloseNormalClosure
				closeText := ""
				var ce *websocket.CloseError
				if errors.As(err, &ce) {
					closeCode = ce.Code
					closeText = ce.Text
				}
				_ = clientConn.WriteMessage(websocket.CloseMessage,
					websocket.FormatCloseMessage(closeCode, closeText))
				return
			}
			if err := clientConn.WriteMessage(msgType, msg); err != nil {
				return
			}
		}
	}()

	<-done
	logger.Info("Wirelogs session ended")
}

// resolvePlane resolves the data plane for a component+environment without
// requiring a specific pod (unlike exec). The flow filter targets pods by label.
func (h *WirelogsHandler) resolvePlane(ctx context.Context, namespace, componentName, project string, envName *string) (execPlaneInfo, error) {
	comp := &openchoreov1alpha1.Component{}
	if err := h.k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: componentName}, comp); err != nil {
		return execPlaneInfo{}, fmt.Errorf("component %q not found: %w", componentName, err)
	}

	if *envName == "" {
		if project == "" {
			return execPlaneInfo{}, fmt.Errorf("--project or --environment is required")
		}
		resolved, err := resolveLowestEnvironment(ctx, h.k8sClient, namespace, project)
		if err != nil {
			return execPlaneInfo{}, err
		}
		*envName = resolved
	}

	env := &openchoreov1alpha1.Environment{}
	if err := h.k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: *envName}, env); err != nil {
		return execPlaneInfo{}, fmt.Errorf("environment %q not found: %w", *envName, err)
	}
	if env.Spec.DataPlaneRef == nil {
		return execPlaneInfo{}, fmt.Errorf("environment %q has no data plane reference", *envName)
	}

	dpResult, err := controller.GetDataPlaneFromRef(ctx, h.k8sClient, env.Namespace, env.Spec.DataPlaneRef)
	if err != nil {
		return execPlaneInfo{}, fmt.Errorf("failed to resolve data plane: %w", err)
	}

	plane := resolveExecPlaneInfo(dpResult)
	if plane.planeID == "" {
		return execPlaneInfo{}, fmt.Errorf("failed to determine plane ID for environment %q", *envName)
	}
	return plane, nil
}

// buildGatewayWirelogsURL constructs the WebSocket URL for the gateway wirelogs endpoint.
func (h *WirelogsHandler) buildGatewayWirelogsURL(plane execPlaneInfo, component, environment, namespace string) (string, error) {
	u, err := url.Parse(h.gatewayURL)
	if err != nil {
		return "", err
	}

	switch u.Scheme {
	case "https":
		u.Scheme = "wss"
	case "http":
		u.Scheme = "ws"
	}

	u.Path = fmt.Sprintf("/api/wirelogs/%s/%s/%s/%s",
		plane.planeType, plane.planeID, plane.crNamespace, plane.crName)

	q := u.Query()
	q.Set("component", component)
	q.Set("environment", environment)
	q.Set("namespace", namespace)
	u.RawQuery = q.Encode()

	return u.String(), nil
}
