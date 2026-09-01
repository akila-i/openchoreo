// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/openchoreo/openchoreo/internal/observer/api/gen"
	observerAuthz "github.com/openchoreo/openchoreo/internal/observer/authz"
	"github.com/openchoreo/openchoreo/internal/observer/service"
	"github.com/openchoreo/openchoreo/internal/observer/types"
)

// GetPlatformLogs handles GET /api/v1alpha1/platform-logs
func (h *Handler) GetPlatformLogs(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()

	req := types.PlatformLogsQueryRequest{
		PlaneKind:      types.PlaneKind(query.Get("planeKind")),
		PlaneName:      query.Get("planeName"),
		PlaneNamespace: query.Get("planeNamespace"),

		ClusterInstance: query.Get("clusterInstance"),
		// Multi-value filters are repeated query parameters, so read every occurrence.
		Namespaces:     query["namespace"],
		PodNames:       query["podName"],
		ContainerNames: query["containerName"],

		StartTime: query.Get("startTime"),
		EndTime:   query.Get("endTime"),

		Labels:       query.Get("labels"),
		SearchPhrase: query.Get("searchPhrase"),
		SortOrder:    query.Get("sortOrder"),
	}

	if rawLimit := query.Get("limit"); rawLimit != "" {
		limit, err := strconv.Atoi(rawLimit)
		if err != nil {
			h.writeErrorResponse(w, http.StatusBadRequest, gen.BadRequest, "", "limit must be an integer")
			return
		}
		req.Limit = limit
	}

	if err := ValidatePlatformLogsQueryRequest(&req); err != nil {
		h.logger.Debug("Platform logs request validation failed", "error", err)
		h.writeErrorResponse(w, http.StatusBadRequest, gen.BadRequest, "", err.Error())
		return
	}

	if h.platformLogsService == nil {
		h.logger.Error("Platform logs service is not initialized")
		h.writeErrorResponse(w, http.StatusInternalServerError, gen.InternalServerError,
			types.ErrorCodeV1PlatformLogsServiceNotReady, "Platform logs service is not initialized")
		return
	}

	result, err := h.platformLogsService.QueryPlatformLogs(r.Context(), &req)
	if err != nil {
		h.writePlatformLogsError(w, err)
		return
	}

	h.writeJSON(w, http.StatusOK, result)
}

// writePlatformLogsError maps platform logs service errors to HTTP responses.
func (h *Handler) writePlatformLogsError(w http.ResponseWriter, err error) {
	if errors.Is(err, observerAuthz.ErrAuthzForbidden) {
		h.writeErrorResponse(w, http.StatusForbidden, gen.Forbidden, "", "Access denied")
		return
	}
	if errors.Is(err, observerAuthz.ErrAuthzUnauthorized) {
		h.writeErrorResponse(w, http.StatusUnauthorized, gen.Unauthorized, "", "Unauthorized")
		return
	}

	errorCode := types.ErrorCodeV1PlatformLogsInternalGeneric
	message := "Failed to retrieve platform logs"
	switch {
	case errors.Is(err, service.ErrScopeAuthFailed):
		errorCode = types.ErrorCodeV1ScopeAuthFailed
		message = "Failed to authenticate plane resolution request"
	case errors.Is(err, service.ErrPlatformLogsResolvePlane):
		errorCode = types.ErrorCodeV1PlatformLogsResolverFailed
		message = "Failed to resolve the requested plane"
	case errors.Is(err, service.ErrPlatformLogsRetrieval):
		errorCode = types.ErrorCodeV1PlatformLogsRetrievalFailed
		message = "Failed to retrieve logs from the logs adapter"
	}

	h.logger.Error("Failed to query platform logs", "error", err)
	h.writeErrorResponse(w, http.StatusInternalServerError, gen.InternalServerError, errorCode, message)
}
