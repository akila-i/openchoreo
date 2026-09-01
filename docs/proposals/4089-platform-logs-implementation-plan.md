# Platform (system-component) logs — implementation plan

**Authors**:
_@akila-i_

**Reviewers**:
_TBD_

**Created Date**:
_2026-08-31_

**Status**:
_Submitted_

**Related Issues/PRs**:
Epic [#4089](https://github.com/openchoreo/openchoreo/issues/4089) ·
Proposal discussion [#4501](https://github.com/openchoreo/openchoreo/discussions/4501) ·
Sub-tasks [#4554](https://github.com/openchoreo/openchoreo/issues/4554),
[#4556](https://github.com/openchoreo/openchoreo/issues/4556),
[#4557](https://github.com/openchoreo/openchoreo/issues/4557),
[#4559](https://github.com/openchoreo/openchoreo/issues/4559),
[#4560](https://github.com/openchoreo/openchoreo/issues/4560)

---

## Progress

| WS | Issue | Status | Commit |
|---|---|---|---|
| WS1 — identity labels on plane pods | #4554 | **Done** | `d22d8428f` |
| WS2 — `platformObservabilityPlaneRef` + CP discoverability | #4559 | **Done** | `e08cb3c64` |
| WS3 — observer + adapter contracts | #4556 / #4557 | **Done** | `33e95a55a` |
| WS4 — observer implementation + authz | #4556 | **Done** | `7b76ca466` |
| WS5 — `query_platform_logs` MCP tool | #4560 | Not started | |

---

## Summary

OpenChoreo's observability plane serves logs for **user workloads** (`dp-*` pods) and **workflow runs**
(`workflow-*` pods). It has no first-class way to observe OpenChoreo's *own* pods — the control-plane
`controller-manager` / `openchoreo-api` / `cluster-gateway`, the per-plane `cluster-agent`s, the
observability-plane observer / sre-agent / finops-agent, and the kgateway proxies.

This document is the implementation plan for the **`openchoreo/openchoreo` half** of epic #4089:
identity labels on platform pods, the plane-CR discoverability field, the
`GET /api/v1alpha1/platform-logs` observer endpoint plus its adapter contract, `platformlogs:view`
authorization, and the `query_platform_logs` MCP tool.

It is an implementation plan, not a design proposal — the design was agreed in discussion #4501. It
records the decisions that were still open after that discussion, and the concrete file-level work each
one implies.

---

## Motivation

To debug a stuck reconcile, a failing `openchoreo-api`, or a crash-looping `cluster-agent`, an operator
today drops to `kubectl logs` / `kubectl top pod` / `kubectl get events` on each cluster. That is
inconsistent with the workload experience, gives no single pane across a multi-cluster topology,
requires cluster credentials, and is invisible to anyone without direct `kubectl` access.

It is also the prerequisite for the use case the feature originates from: platform-engineering teams
asking to set alerts that monitor OpenChoreo itself.

The feature is **opt-in**. Platform logs (controllers, API server) are noisy and can fill a logs backend
quickly, so nothing is collected until a platform engineer enables it.

---

## Goals

- Platform pods across all four planes carry enough identity to be attributed to a plane at query time.
- One observer endpoint serves platform logs for a selected plane, reusing the existing adapter
  architecture.
- Operator/admin-scoped authorization, distinct from workload log access.
- The same contract is available to the portal (later) and to MCP clients.

---

## Non-Goals

**Signals.** Logs only. Metrics and Kubernetes events follow the same identity model and endpoint shape
but are not implemented here — the epic title should be narrowed, or they should be split into their own
epic. Distributed traces are out of scope entirely: platform components are not instrumented for tracing.

**Coverage boundary.** Only Helm-installed workloads. `kube-system` — CoreDNS, kube-proxy, the CNI, the
API server — is a layer below OpenChoreo and is not collected. Static pods could not be labelled anyway,
and on GKE/EKS the provider's addon manager would revert pod-label patches. The UI must say where this
boundary is, or a quiet view reads as a healthy cluster.

**Deferred from this slice**, each additive to the contract later: a `level` filter ·
`totalRelation: eq|gte` · `include=facets,histogram` · cursor pagination · per-plane ABAC.

Facets are the one with a sequencing consequence — they should land before the portal work, or the
portal ships one request per picker and someone unpicks it later.

**Other repositories.** Named in [Impact](#impact) but not planned here.

---

## Impact

| Area | Change |
|---|---|
| Helm charts (all 4 planes) | New pod-template labels ⇒ **one rolling restart** of the control plane, agents and gateways on upgrade. Belongs in the release notes |
| `api/v1alpha1` | New optional `platformObservabilityPlaneRef` on six plane specs. Additive, no migration |
| openchoreo-api | One new authenticated metadata endpoint; six plane spec schemas gain a field |
| Observer | One new endpoint, one new service, one new authz action |
| Authorization | New cluster-scoped `platformlogs:view`; the `observer-resource-reader` service role gains six plane read actions |
| Adapter contract | New path on `openapi/observability-logs-adapter-api.yaml` — **see the cross-repo hazard in [WS3](#ws3--contracts-spec-half-of-4556-and-4557)** |

### Out of scope here, named so nothing silently drops

| Item | Repo |
|---|---|
| Namespace-gated collection + platform index/retention for the three Fluent Bit modules (#4555) | community-modules |
| Azure DCR recipe; GCP Log Router sink + `_Default` exclusion; required `clusterInstance` on every collector | community-modules |
| `platform-logs/query` implementations in all five adapters (#4557) — the **spec** is in WS3 | community-modules |
| Portal platform-logs view (#4558) and plane entity tabs | backstage-plugins |
| External component opt-in docs (#4562); per-module namespace-gate recipes; the upgrade note | openchoreo.github.io |

---

## Design

### Decisions this plan is built on

| Decision | Value |
|---|---|
| Release scope | Thin vertical slice for v1.3.0; everything else sequenced after |
| Authorization | Trust the chart-set `openchoreo.dev/plane` label. One cluster-scoped `platformlogs:view`. The real boundary is the namespace allowlist keeping `dp-*` / `workflow-*` out of the platform index |
| planeName → planeID | **Observer resolves via openchoreo-api.** No new plane endpoint needed — the six plane GETs already exist and already return `spec.planeID` |
| `planeKind` enum | 7 CR kinds + `Other`; the observer collapses to the 4 label values internally |
| Response extras | `PlatformLogsResponse` + `403` only |
| Collection gate | Namespace allowlist in the observability module, **not** a pod label. Pod labels are attribution only |

> **Note on epic #4089's body.** Its layer-2 section still describes gating collection with an
> `Exclude_Path` on `*_dp-*_*.log`. That was superseded by the namespace-allowlist gate agreed in the
> Aug 25 comment on discussion #4501, and this plan follows the namespace gate.

### Identity model

Identity travels on **pod labels**, set by the chart that creates the pod, landing at
`kubernetes.labels.*` via the collector's Kubernetes metadata filter — the same mechanism workload logs
already use, so no new collector capability is needed.

| Label | Value | Set on |
|---|---|---|
| `openchoreo.dev/plane` | `controlplane` \| `dataplane` \| `workflowplane` \| `observabilityplane` | Plane-owned pods. **Omitted** for external/shared components, so `!openchoreo.dev/plane` selects them |
| `openchoreo.dev/plane-id` | the install's `planeID` | Plane-owned pods, except the control plane (a singleton) |
| `openchoreo.dev/cluster-instance` | e.g. `cluster1` | Stamped by the **collector**, not the chart — it is the one component that is genuinely one-per-cluster |

`(cluster-instance, plane-id)` is the unique key: `planeID` defaults to `default` in every plane chart,
so two clusters running defaults would otherwise be indistinguishable.

**Plane CR → pods is resolved at query time, not stamped.** Multiple plane CRs may share one `planeID`
(explicitly supported — see the field's doc comment on the plane types), so no single CR name can be
written onto a pod. Pods carry physical identity; the observer maps the selected plane CR to its
`spec.planeID`. N CRs sharing a planeID resolve to the same filter, which is correct — they share pods.

### Reuse map — what already exists

| Need | Existing thing to reuse |
|---|---|
| Chart-wide extra labels | `global.commonLabels` in all 4 charts — but it reaches **object** `metadata.labels` only, never pod templates |
| Label helpers | `<chart>.labels` / `.selectorLabels` / `.componentLabels` / `.componentSelectorLabels`, byte-identical across all 4 `_helpers.tpl` |
| Gateway pod labels | `install/helm/openchoreo-data-plane/values.yaml:80-89` already sets `gateway.infrastructure.labels` (`openchoreo.dev/system-component: gateway`) — the precedent |
| Plane ref type | `ObservabilityPlaneRef` / `ClusterObservabilityPlaneRef` (`api/v1alpha1/types.go:182-233`), typed `{Kind, Name}`, no namespace |
| Plane ref resolution | `internal/controller/reference.go` — `GetObservabilityPlaneFromRef:261`, `getDefaultObservabilityPlane:297`, `clusterObsRefToObsRef:324`, `ObservabilityPlaneResult.GetPlaneID:226` |
| planeID over HTTP | `openapi/openchoreo-api.yaml` — all six plane GETs exist, and all six `*PlaneSpec` schemas already expose `planeID` (`:7691, 7823, 7988, 8073, 8152, 8252`) |
| Observer → OC API client | `internal/observer/service/uid_resolver.go` — OAuth client-credentials, token cache, retry (`fetchResourceUID:161`, `doFetchResourceUID:183`) |
| GET endpoint with query params | `internal/observer/api/handlers/finops.go` + reusable `components.parameters` at `openapi/observer-api.yaml:910-972` |
| Authz decorator | `internal/observer/service/logs_authz.go` (54 lines) + `internal/observer/authz/helpers.go:95 CheckAuthorization` |
| Action registry | `internal/authz/core/actions.go` — consts `:249-282`, registry entries `:516-548` |
| MCP tool template | `internal/observer/mcp/server.go:45-81` (`query_component_logs`), `createSchema` / `stringProperty` / `limitLogsProperty` helpers |
| End-to-end precedent | commit `0d6c4cb73` (FinOps observer endpoint) — 30 files, the exact footprint of "new observer endpoint + adapter contract + authz + helm" |

---

### WS1 — Identity labels on platform pods (#4554)

**Status: done** — commit `d22d8428f`. See [As implemented](#ws1-as-implemented) for the
three things that differed from the plan below.

**Two hazards, both confirmed.** (a) All 13 Deployments render `spec.selector.matchLabels` and
`spec.template.metadata.labels` from the *same* expression, so anything added via a selector helper
lands in the immutable selector and makes `helm upgrade` **fail** on existing installs rather than
adding the label. (b) There is no pod-labels-only seam today.

**Approach: one new helper per chart, one `include` line per pod template. Selectors untouched.** Do not
extend `selectorLabels` / `componentSelectorLabels`, and do not route this through `global.commonLabels`.

1. Add to each `install/helm/openchoreo-*/templates/_helpers.tpl`:

```gotemplate
{{/*
Platform identity labels for platform-logs attribution.
POD TEMPLATES ONLY - never spec.selector.matchLabels (immutable; would break helm upgrade).
*/}}
{{- define "openchoreo-data-plane.platformIdentityLabels" -}}
openchoreo.dev/plane: dataplane
openchoreo.dev/plane-id: {{ .Values.clusterAgent.planeID | default .Release.Name | quote }}
{{- end }}
```

- The control plane emits **only** `openchoreo.dev/plane: controlplane` — it is a singleton and the chart
  has no `clusterAgent` block, so no `planeID` exists there.
- Data / workflow / observability planes emit both. The `planeID` expression must match the agent's argv
  verbatim (`templates/cluster-agent/deployment.yaml:63`,
  `{{ .Values.clusterAgent.planeID | default .Release.Name }}`) or the logs and the gateway connection
  disagree about which plane they belong to.
- Unconditional — no new values key, so no `values.schema.json` churn. Labels alone collect nothing;
  collection is gated by namespace in the module, which is also why the originally proposed
  `observability.enabled` toggle is dropped rather than renamed.

2. Add one line to each of the **14** pod templates. Pattern B (literal component + `selectorLabels`):

```yaml
      labels:
        app.kubernetes.io/component: api-server
        {{- include "openchoreo-control-plane.selectorLabels" . | nindent 8 }}
        {{- include "openchoreo-control-plane.platformIdentityLabels" . | nindent 8 }}
```

Pattern A (`componentSelectorLabels` dict form) takes the same extra line with `.` unchanged.

Full list — **CP**: `controller-manager`, `openchoreo-api`, `backstage`, `cluster-gateway`,
`event-forwarder`, `portal-assistant` deployments + `templates/authz/bootstrap-job.yaml` (Job pod
template; its selector is system-generated, so it is safe). **DP**: `cluster-agent`. **WP**:
`cluster-agent`. **OP**: `cluster-agent`, `controller-manager`,
`observer/observer-deployment.yaml`, `sre-agent`, `finops-agent`.

3. **Gateway CRs** — kgateway creates those pods from the `Gateway` CR, so no chart helper reaches them.
   Merge identity labels into `spec.infrastructure.labels` in all three `templates/gateway/gateway.yaml`
   (CP, DP, OP). The current shape is a blanket
   `{{- with .Values.gateway.infrastructure }}{{- toYaml . | nindent 4 }}{{- end }}`; replace it with a
   merge that emits `labels` unconditionally and preserves any other `infrastructure` keys (sprig `omit`
   / `mustMerge`). Only DP has a non-empty default today.

   **Verify on k3d before merging** that kgateway applies `infrastructure.labels` to the generated proxy
   Deployment's pod template and *not* to its selector — if it reaches the selector, the same
   immutability failure applies to gateway upgrades.

4. **Known gaps to state in the PR, not fix here:** the workflow-plane `argo-workflows` subchart pods
   (label via the subchart's own `podLabels` values if supported) and community-module pods installed
   into the OP namespace (fluent-bit, OpenSearch). Unlabelled pods in a collected namespace surface under
   `Other / Shared` — correct behaviour, worth documenting.

5. **Upgrade note:** this mutates pod templates, so it triggers one rolling restart of the control plane,
   agents and gateways.

<a id="ws1-as-implemented"></a>
#### As implemented

Three things the plan did not anticipate:

1. **The Gateway merge needed a variable, not `with`.** Emitting identity labels while preserving
   values-supplied ones without producing duplicate YAML keys took
   `merge (dict) ($infra.labels | default dict) (include "<chart>.platformIdentityLabels" . | fromYaml)`,
   so a values-supplied label wins on conflict. A consequence worth knowing: `spec.infrastructure`
   is now always emitted on the control-plane and observability-plane Gateways, where it was
   previously omitted because `.Values.gateway.infrastructure` defaults to `{}`.
2. **The argo-workflows subchart pods stay unattributed, permanently.** Subchart values cannot
   template `planeID`, so those pods could only carry `openchoreo.dev/plane` with no id — and that is
   worse than nothing when two workflow planes share a cluster, because it asserts a plane without
   saying which instance. They surface as unattributed, which is accurate. Same for pods the
   community observability modules install into a plane namespace.
3. **`helm template` on default values fails `validate.yaml`** (placeholder `.invalid` domains,
   required secret names), so verification runs against `test/e2e/k3d/values-{cp,dp,wp,op}.yaml` plus
   `--set`s to enable the optional components. The workflow-plane chart also needs
   `helm repo add argo https://argoproj.github.io/argo-helm` before `helm dependency build`.

**Still open — the one check that needs a cluster.** Whether kgateway copies
`spec.infrastructure.labels` onto the generated proxy Deployment's *selector* rather than only its pod
template. `helm template` cannot answer this; if it does reach the selector, gateway upgrades hit the
same immutability failure this workstream exists to avoid. Verify before the PR merges.

---

### WS2 — `platformObservabilityPlaneRef` + control-plane discoverability (#4559)

**Status: done** — commit `e08cb3c64`. See [As implemented](#ws2-as-implemented); item 5 turned out
smaller than planned and the fallback rule below needed one correction.

1. **CRD types** — add to all six specs in `api/v1alpha1/`:
   `dataplane_types.go`, `workflowplane_types.go`, `observabilityplane_types.go` get
   `PlatformObservabilityPlaneRef *ObservabilityPlaneRef`;
   `clusterdataplane_types.go`, `clusterworkflowplane_types.go`, `clusterobservabilityplane_types.go`
   get `*ClusterObservabilityPlaneRef` **with the same XValidation guard** already used at
   `clusterdataplane_types.go:38-49`. Note `ObservabilityPlaneSpec` has no ref fields today — this is its
   first (an OP's own platform logs may go to a different OP, or to itself).

2. **Resolution helper** — extend `internal/controller/reference.go` with
   `GetPlatformObservabilityPlaneFromRef`, reusing `clusterObsRefToObsRef` and
   `getDefaultObservabilityPlane`. **Fallback order: `platformObservabilityPlaneRef` →
   `observabilityPlaneRef` → the `default` OP.** Falling back to the existing ref means a fresh install
   works with no new configuration; state that in the field's doc comment.

3. **Regenerate** — `make manifests` → `make generate` → `make helm-generate`. Chart CRD copies under
   `install/helm/openchoreo-control-plane/crds/` are produced by `tools/helm-gen/crd.go`; never hand-edit.

4. **openchoreo-api spec** — `openapi/openchoreo-api.yaml` is hand-maintained, so add
   `platformObservabilityPlaneRef` to the six `*PlaneSpec` schemas by hand, then `make openapi-codegen`.

5. **Control-plane metadata endpoint** — the CP has no CR. Follow the `oauth_metadata.go` chain exactly:
   - `install/helm/openchoreo-control-plane/values.yaml` →
     `openchoreoApi.config.platformObservability.observabilityPlaneRef.{kind,name}`
   - `templates/openchoreo-api/configmap.yaml` → new `platform_observability:` koanf section
   - `internal/openchoreo-api/config/` → new section struct + `Config` field
   - `openapi/openchoreo-api.yaml` → `GET /api/v1/platform-observability`, `tags: [Operations]`.
     **Keep it authenticated** (no `security: []` — unlike the OAuth metadata document, this is operator
     information). No new authz action; it discloses only a ref.
   - `internal/openchoreo-api/api/handlers/` → new handler method on `*Handler` reading `h.Config`
   - Response: `{planeKind: ControlPlane, observabilityPlaneRef: {kind, name}}`. The portal resolves the
     ref to an `observerURL` through the existing observability-plane GETs — do not duplicate that here.

6. **`planeID` uniqueness** — two data planes installed into one cluster with default values both get
   `planeID: default` and become indistinguishable, which is exactly the topology this epic exists to
   support. The charts already carry a `| default .Release.Name` expression in templates while
   `values.yaml` hardcodes `default`; changing the values default to empty so the release-name fallback
   actually fires is the smallest fix. **Treat as a separate PR** — it changes existing installs' plane
   identity.

<a id="ws2-as-implemented"></a>
#### As implemented

1. **The fallback rule is wrong for observability planes.** `platformRef → observabilityPlaneRef →
   "default"` is right for data and workflow planes, but an observability plane falling back to a
   plane named `default` would fail on any install whose only observability plane is named something
   else. `ObservabilityPlaneResult.GetPlatformObservabilityPlane` returns **the plane itself** when
   no ref is set. A test asserts this with no `default` plane present, so a regression to the generic
   chain fails loudly.
2. **WS4 needs no new openchoreo-api endpoint.** The plan assumed plane→planeID resolution would
   need one. It does not: all six plane GETs already exist and their `*PlaneSpec` schemas already
   return `planeID`, so WS4's resolver is a new method against existing paths. Only the control-plane
   metadata endpoint was genuinely new here.
3. **The endpoint reports `enabled` as well as the ref**, so a client can tell "platform
   observability is not configured" apart from "configured but returning nothing".
4. **`make mockery-gen` is mandatory after any `openapi/openchoreo-api.yaml` change.** A new path
   adds a method to the generated `ClientWithResponsesInterface`, which breaks
   `internal/occ/resources/client/mocks/` — a package far from the change, caught only by
   `make lint` (as a `typecheck` failure), not by `go build ./...`.
5. **`values.schema.json` rejects `type` alongside `enum`.** The helm-schema tool fails with
   "cannot use both 'enum' and 'type' in the same schema"; all 42 existing enums in that file omit
   `type`, so the annotation must too.

**Not done here, deliberately:** the `planeID`-uniqueness change (item 6 below). It alters the plane
identity of existing installs, so it belongs in its own PR rather than riding along with an additive
field.

---

### WS3 — Contracts (spec half of #4556 and #4557)

**Status: done** — commit `33e95a55a`. See [As implemented](#ws3-as-implemented).

Land the **observer spec first**, then the adapter spec, so the shapes agree.

#### `openapi/observer-api.yaml` — `GET /api/v1alpha1/platform-logs`

Declare parameters as reusable `components.parameters` entries following the FinOps precedent
(`:910-972`).

This is a deliberate divergence from `POST /api/v1/logs/query`: platform scope is a fixed set of
Kubernetes coordinates, so there is nothing for a `oneOf` to discriminate between, and a GET gives the
portal a shareable, cacheable URL — which is what the permalink and deep-link behaviour rests on.
**State this in the path description** so the asymmetry reads as a decision. The existing
`ComponentSearchScope` / `WorkflowSearchScope` union stays untouched.

| Param | Notes |
|---|---|
| `planeKind` | **required**, enum: `ControlPlane, DataPlane, ClusterDataPlane, WorkflowPlane, ClusterWorkflowPlane, ObservabilityPlane, ClusterObservabilityPlane, Other` |
| `planeName` | required unless `planeKind` ∈ {`ControlPlane`, `Other`} — enforce server-side with a 400 |
| `planeNamespace` | required for the three namespaced kinds. **Named distinctly on purpose**: `namespace` below means the platform pod's namespace, a different thing |
| `namespace`, `podName`, `containerName` | repeatable, each with `maxItems: 20`, enforced server-side with a 400 (not truncated) |
| `clusterInstance` | optional; the only scope `Other / Shared` records have once `plane` is absent |
| `startTime`, `endTime` | **required**, absolute RFC3339 UTC |
| `limit` | 1–1000, default 100 |
| `sortOrder` | `asc` \| `desc`, default `desc` |
| `labels` | Kubernetes label-selector string, `maxLength: 256` |
| `searchPhrase` | `maxLength: 256` |

Every list needs a declared `maxItems` because a query string has length limits a request body would not
— the same class of concern that produced the 256-character `searchPhrase` cap.

Schemas:

- `PlatformLog` — camelCase, and use **`log`** for the message field to match `ComponentLogEntry` /
  `WorkflowLogEntry` (the discussion said `message`; consistency wins):
  `{timestamp, log, level, planeKind, planeId, clusterInstance, namespaceName, podName, containerName}`.
- `PlatformLogsResponse` — `{tookMs, total, logs}`, all required, mirroring `LogsQueryResponse`. Point
  the `200` here, not at a bare array.
- Responses `200, 400, 401, 403, 500`, all errors `$ref: ErrorResponse`. `403` is separate from `401`
  because a plane-level authorization denial is a different remedy and the UI shows different copy.

#### `openapi/observability-logs-adapter-api.yaml` — `POST /api/v1alpha1/platform-logs/query`

**POST, not GET**, and a separate path rather than a third `searchScope` variant. Reasons: the observer
sends *resolved physical* identity rather than names; all five modules discriminate the existing
`searchScope` union on "`workflowRunName` is non-nil", so a third member would need a new discriminator
in five places; and there is no gateway URL-length concern on an in-cluster hop.

Request `PlatformLogsQueryRequest`:

```jsonc
{
  "startTime": "…", "endTime": "…", "limit": 100, "sortOrder": "desc",
  "labels": "…", "searchPhrase": "…",
  "scope": {
    "plane": "controlplane|dataplane|workflowplane|observabilityplane", // collapsed; omitted for Other
    "unattributed": false,                                              // true => !openchoreo.dev/plane
    "planeId": "…", "clusterInstance": "…",
    "namespaces": [], "podNames": [], "containerNames": []
  }
}
```

Response `PlatformLogsResponse` — same shape as the observer's.

> **Cross-repo hazard to flag on the PR.** All five community-modules Makefiles fetch this spec from
> `main`
> (`SPEC := https://raw.githubusercontent.com/openchoreo/openchoreo/main/openapi/observability-logs-adapter-api.yaml`),
> and it drives a `strict-server` interface. A new path adds a method to `StrictServerInterface`, so each
> module fails to compile the next time it regenerates until it implements it. Land 501 stubs in the
> modules promptly after merging this spec.

<a id="ws3-as-implemented"></a>
#### As implemented

1. **Two enums, not one.** `planeKind` on the request accepts the seven CR kinds plus `Other`,
   because a plane *name* is ambiguous without knowing which CR it names. But the pod label
   vocabulary is only four values, so `PlatformLog.planeKind` and the adapter's `scope.plane` are
   typed to the collapsed four and the observer does the collapse. The plan treated this as one
   enum; it is two, and WS4 owns the mapping.
2. **`planeNamespace` vs `namespace`.** Called out in the plan and worth repeating, because the
   generated `GetPlatformLogsParams` now has both and they are one letter apart in meaning: the
   plane CR's namespace, and the platform pod's namespace.
3. **The adapter path declares no `403`**, unlike `/api/v1/logs/query`. Authorization for platform
   scope happens at the observer, and the real boundary is the namespace allowlist on the collector
   rather than the pod labels. Omitting 403 reinforces that instead of inviting five module authors
   to each invent one. The adapter description states the rule outright.
4. **What WS4 will call**: `logsadapterclientgen.ClientWithResponses.QueryPlatformLogsWithResponse`,
   with `PlatformLogsQueryRequest` / `PlatformSearchScope`. The observer's own
   `GetPlatformLogsParams` documents the request contract but is not wired into routing, since the
   observer hand-registers routes rather than using the generated server.
5. **`make mockery-gen` again.** Same trap as WS2 — regenerate after touching either spec.

---

### WS4 — Observer implementation + authorization (#4556)

**Status: done** — commit `7b76ca466`. See [As implemented](#ws4-as-implemented).

The footprint mirrors `0d6c4cb73`. Note the observer does **not** use the generated server — routes are
hand-registered in `cmd/observer/main.go` and logs use hand-written types in `internal/observer/types/`
— so follow that, not `server.gen.go`.

**New files**

- `internal/observer/types/platformlogs.go` — `PlatformLogsQueryRequest`, `PlatformLog`,
  `PlatformLogsResponse`; error codes appended to `internal/observer/types/errors.go` (`OBS-V1-PL-*`,
  alongside `OBS-V1-L-01..05`)
- `internal/observer/api/handlers/platformlogs.go` — `GetPlatformLogs`, shaped like `finops.go`; read
  repeatables with `r.URL.Query()["namespace"]`; map `ErrAuthzForbidden` → 403,
  `ErrAuthzUnauthorized` → 401 exactly as `logs.go` does
- `internal/observer/service/platform_logs.go` — `PlatformLogsService`: resolve
  (`planeKind`, `planeNamespace`, `planeName`) → `planeID`, collapse the 7 CR kinds to the 4 label
  values, map `Other` → `unattributed: true`, call the adapter
- `internal/observer/service/platform_logs_adapter.go` — use the **generated** `logsadapterclientgen`
  client (as `finops_adapter.go` does), not the hand-rolled structs in `logs_adapter.go`
- `internal/observer/service/platform_logs_authz.go` — decorator; `ActionViewPlatformLogs` with an
  **empty `ResourceHierarchy`** (cluster scope). The Casbin PDP decides on `Resource.Hierarchy` and
  ignores `Resource.ID`, so do not put `planeName` in the hierarchy and expect it enforced — per-plane
  ABAC is explicitly out of scope for this slice

**Edited files**

- `internal/observer/service/uid_resolver.go` — add `GetPlaneID(ctx, kind, namespace, name)` hitting the
  existing six OC API plane GETs. `fetchResourceUID` extracts `metadata.uid`; refactor its
  auth / retry / token machinery into a shared fetch that returns the decoded body so the new method can
  read `spec.planeID` without duplicating it
- `internal/observer/service/interfaces.go` — `PlatformLogsQuerier`
- `internal/observer/api/handlers/handler.go` — new service field + `NewHandler` param
- `internal/observer/api/handlers/validations.go` — `ValidatePlatformLogsQueryRequest`: reuse
  `ValidateTimeRange` (`maxQueryTimeRange = 30d`), `ValidateAndSetLimit`, `ValidateAndSetSortOrder`; add
  the `maxItems` caps, the 256-character caps, and planeName / planeNamespace requiredness per kind
- `internal/authz/core/actions.go` — `ActionViewPlatformLogs = "platformlogs:view"` const near `:249-282`
  and registry entry `{Name: ActionViewPlatformLogs, LowestScope: ScopeCluster, IsInternal: false}` near
  `:516-548`. No `conditionRegistry` entry — no CEL attributes apply at cluster scope
- `internal/observer/authz/constants.go` — matching `Action` constant
- `cmd/observer/main.go` — construct the service, wrap with authz beside the other `New*ServiceWithAuthz`
  calls (`:218-231`), pass into both `apihandler.NewHandler` and `observermcp.NewMCPHandler`, and register
  `api.HandleFunc("GET /api/v1alpha1/platform-logs", newAPIHandler.GetPlatformLogs)`
- `.mockery.yaml` — add `PlatformLogsQuerier` to the `internal/observer/service` block (`:138-150`)
- `install/helm/openchoreo-control-plane/values.yaml` — **two grants, both easy to miss:**
  1. `platformlogs:view` on the operator / PE roles (`admin` already has `*`)
  2. the six plane view actions (`dataplane:view`, `clusterdataplane:view`, `workflowplane:view`,
     `clusterworkflowplane:view`, `observabilityplane:view`, `clusterobservabilityplane:view`) added to
     the `observer-resource-reader` role at `:1303-1309` — without these, planeID resolution 403s

No new observer config: the endpoint reuses `LOGS_ADAPTER_URL` / `UID_RESOLVER_*`.

<a id="ws4-as-implemented"></a>
#### As implemented

1. **The resolver refactor was load-bearing, not cosmetic.** `fetchResourceUID` decoded
   `metadata.uid` *inside* the retry loop, so plane resolution could not reuse the OAuth, retry and
   401-invalidation machinery without duplicating it. It is now `fetchResource` returning the raw
   body, with `fetchResourceUID` and `GetPlaneID` as thin decoders over it. No behaviour change for
   existing callers.
2. **`GetPlaneID` falls back to the CR name when `spec.planeID` is empty**, mirroring the charts'
   `planeID | default .Release.Name`. Without it, a plane installed with no explicit planeID
   resolves to `""` and the query silently matches nothing — the worst kind of failure here, since
   an empty log view looks identical to a healthy quiet plane.
3. **Naming a plane on `ControlPlane` or `Other` is a 400, not ignored.** Ignoring it would return
   control-plane logs to a caller who asked for something else.
4. **The authz shape is pinned by a self-explaining test.**
   `TestPlatformLogsAuthz_UsesClusterScopeWithEmptyHierarchy` asserts the hierarchy stays empty,
   because the Casbin PDP decides on `Resource.Hierarchy` — a plane name there would imply
   per-plane enforcement the data cannot back. That test is where the decision is recorded if
   someone later wants real per-plane ABAC.
5. **Both role grants landed**: `platformlogs:view` on `sre` and `platform-engineer` (`admin`
   already has `*`; deliberately not `developer`, which is tenant-scoped, nor `rca-agent`, which
   does workload RCA), and the six plane view actions on `observer-resource-reader`.

**Verification gotcha:** `make lint 2>&1 | tail -n` swallows the exit code — `$?` is `tail`'s.
Capture `make lint`'s status directly, or a red lint reads as green.

---

### WS5 — `query_platform_logs` MCP tool (#4560)

- `internal/observer/mcp/server.go` — tool 14 in `registerTools`, built with `createSchema` /
  `stringProperty` / `arrayProperty` / `limitLogsProperty` / `sortOrderProperty`; args as an anonymous
  struct with snake_case `json:` tags (`plane_kind`, `plane_name`, `plane_namespace`, `namespace`,
  `pod_name`, `container_name`, `cluster_instance`, `start_time`, `end_time`, `search_phrase`, `labels`,
  `limit`, `sort_order`). Required: `plane_kind`, `start_time`, `end_time`
- `internal/observer/mcp/handlers.go` — `QueryPlatformLogs`; add the service to `MCPHandler` and its
  nil-check list in `NewMCPHandler`
- `internal/observer/mcp/helpers.go` — `validatePlatformScope` (planeName / planeNamespace requiredness
  per kind), reusing `setDefaults`
- `internal/observer/mcp/server_test.go` — new `allToolSpecs` entry **and** an `expectedTypes` entry in
  `TestSchemaPropertyTypes`; plus `handlers_test.go` / `helpers_test.go`
- `test/e2e/suites/observability/observer_mcp_test.go:47-63` — `allObserverTools` is **already stale**: it
  lists 11 names and is missing `query_costs` / `query_recommendations` from `69f9093a9`. Update it to the
  full 14

---

### Suggested order

```
WS1 (chart labels) ──┐
WS2 (discoverability)┴─→ WS3 (observer spec → adapter spec) ─→ WS4 (observer + authz) ─→ WS5 (MCP)
```

WS1 and WS2 are independent of each other and of the contracts, so they can go in parallel. WS3's two
specs are strictly ordered. WS5 is blocked on WS4.

---

## Verification

**Static / unit**

```bash
make lint
make test
make code.gen && git diff --exit-code    # catches CRD, values.schema.json and oapi-codegen drift
```

**Chart labels — the selector-immutability check that matters most**

Default values fail `validate.yaml`, so render against the e2e values files and enable the optional
components explicitly. The workflow-plane chart needs its argo dependency fetched first
(`helm repo add argo https://argoproj.github.io/argo-helm && helm dependency build install/helm/openchoreo-workflow-plane`).

```bash
render() { helm template rel "install/helm/openchoreo-$1-plane" -f "test/e2e/k3d/values-$2.yaml" "${@:3}"; }

{ render control cp --set backstage.enabled=true --set backstage.secretName=bs \
      --set portalAssistant.enabled=true --set portalAssistant.llm.secretName=pa \
      --set portalAssistant.llm.modelName=openai:gpt-4o-mini
  render data dp
  render workflow wp
  render observability op --set rca.enabled=true --set finOpsAgent.enabled=true \
      --set rca.openchoreoApiUrl=http://oc-api.test --set finOpsAgent.openchoreoApiUrl=http://oc-api.test
} | yq 'select(.kind=="Deployment" or .kind=="Job" or .kind=="Gateway")
    | [ .kind + "/" + .metadata.name,
        ((.spec.template.metadata.labels // .spec.infrastructure.labels // {})
          | with_entries(select(.key|test("^openchoreo.dev/plane"))) | to_entries
          | map(.key + "=" + .value) | join(" ")),
        ((.spec.selector.matchLabels // {})
          | with_entries(select(.key|test("^openchoreo.dev/plane"))) | length | tostring) ]
    | join("|")'
```

Expect an identity label set on all 14 pod templates and all 3 Gateways, and the third column `0`
everywhere. The two argo-workflows pods are expected to be blank.

Then check `planeID` really threads through — the pod label must equal the agent's argv, or the logs
and the gateway connection disagree about which plane they belong to:

```bash
helm template rel install/helm/openchoreo-data-plane -f test/e2e/k3d/values-dp.yaml \
  --set clusterAgent.planeID=dp-eu-1 \
| yq 'select(.kind=="Deployment") | {
    "pod-label": .spec.template.metadata.labels["openchoreo.dev/plane-id"],
    "argv": (.spec.template.spec.containers[0].args // [] | map(select(test("plane-id"))) | join(""))}'
```

Finally prove the upgrade path on a live install (install on k3d, then `make k3d.update`) — a label
that leaked into a selector fails there and nowhere earlier — and confirm the kgateway proxy pods
picked the labels up:
`kubectl get pod -n openchoreo-data-plane -l openchoreo.dev/plane=dataplane --show-labels`.

**Plane ref and control-plane discoverability**

```bash
make manifests generate                       # CRDs + deepcopy
make helm-generate.openchoreo-control-plane   # chart CRD copies + values.schema.json
make openapi-codegen                          # API models/server/client
make mockery-gen                              # occ client mock gains the new endpoint's method
make lint && make go.test                     # go.test, not `go test ./...`, which lacks KUBEBUILDER_ASSETS
```

Then check the Helm chain renders, and that the schema rejects a bad kind:

```bash
helm template rel install/helm/openchoreo-control-plane -f test/e2e/k3d/values-cp.yaml \
  --set platformObservability.observabilityPlaneRef.name=default \
| yq 'select(.kind=="ConfigMap" and (.metadata.name|test("openchoreo-api-config"))) | .data["config.yaml"]' \
| grep -A3 platform_observability

helm template rel install/helm/openchoreo-control-plane -f test/e2e/k3d/values-cp.yaml \
  --set platformObservability.observabilityPlaneRef.kind=Nonsense   # must fail schema validation
```

**Observer endpoint**

- Handler tests alongside `internal/observer/api/handlers/logs_test.go`: the 400 cases (missing
  `planeName` for a named kind, `maxItems` exceeded, `searchPhrase` > 256, time range > 30d), 401 / 403
  mapping, and the happy path against the mocked `PlatformLogsQuerier`
- Service tests with a stubbed adapter and stubbed resolver: assert the 7 → 4 kind collapse, `Other` →
  `unattributed`, and that `planeNamespace` is not confused with the log-filter `namespace`
- Manual: port-forward the observer and
  `curl -H "Authorization: Bearer $TOKEN" '.../api/v1alpha1/platform-logs?planeKind=ControlPlane&startTime=…&endTime=…'`

**MCP** — `make test` covers the table-driven schema tests; then the tier3 e2e
`test/e2e/suites/observability/observer_mcp_test.go` for the 14-tool assertion.

**Real end-to-end is blocked on a companion community-modules PR** and cannot be closed out from this
repo. Two specifics worth knowing when planning that PR:

- `observability-logs-opensearch/init/setup-opensearch.sh:60-142` sets the `container-logs-*` index
  template to `"dynamic": false` with an OpenChoreo-label allowlist, so platform pod labels are stored but
  **not indexed** — platform queries cannot work on OpenSearch until that template changes.
- `observability-logs-openobserve` has a dynamic schema and is what `make/e2e.mk:1053-1074` already
  installs, so it is the practical first backend for validating this contract.

---

## Appendix

- Epic [#4089](https://github.com/openchoreo/openchoreo/issues/4089) and proposal discussion
  [#4501](https://github.com/openchoreo/openchoreo/discussions/4501), including the Aug 18 meeting notes
  (opt-in decision) and the Aug 25 comment (namespace collection gate).
- Existing contracts touched: `openapi/observer-api.yaml`, `openapi/observability-logs-adapter-api.yaml`,
  `openapi/openchoreo-api.yaml`.
- Closest implementation precedent: commit `0d6c4cb73`, the FinOps observer endpoint.
