# ACL Support for CCEE (SCI / OpenStack) — Design

- **Issue:** [#1569](https://github.com/kyma-project/kyma-infrastructure-manager/issues/1569)
- **Parent:** [#1410](https://github.com/kyma-project/kyma-infrastructure-manager/issues/1410) — Extend ACL feature to support GCP and CCEE (SCI) Hyperscalers
- **Date:** 2026-08-21
- **Classification:** Bounded

## Summary

Extend the existing API server ACL feature to support the CCEE (SCI)
Hyperscaler, which runs on OpenStack. The ACL feature is already implemented
end-to-end for AWS and Azure. Enabling CCEE requires adding
`hyperscaler.TypeOpenStack` to a single provider-type gate.

## Background

The ACL (Access Control List) feature restricts network access to the
Kubernetes API server at the hyperscaler level, using Gardener's `acl`
extension. When enabled for a Runtime, KIM merges three sources of CIDRs into
a single ALLOW rule:

- User-provided CIDRs (`runtime.Spec.Shoot.Kubernetes.KubeAPIServer.ACL.AllowedCIDRs`)
- Operator IPs (from the ACL ConfigMap, key `acl-list.json`)
- The KCP external NAT IP (from the ACL ConfigMap, key `kcp-external-nat-ip.json`)

Access to this feature is guarded by the `api-server-acl-enabled` operator
feature flag and, per provider, by the `AclNeedsToBeEnabled` gate function.

### Why CCEE has no blockers

OpenStack NAT gateways have static IP addresses by default, so no additional
static-IP configuration is required (confirmed by Gardener). This is in
contrast to GCP (issue #1570), which remains blocked pending static IP feature
delivery.

## Current Implementation

Single gate function — `pkg/gardener/shoot/extender/extensions/apiServerACL.go`:

```go
func AclNeedsToBeEnabled(apiServerAclEnabled bool, runtime imv1.Runtime) bool {
	runtimeType := runtime.Spec.Shoot.Provider.Type

	return apiServerAclEnabled &&
		(runtimeType == hyperscaler.TypeAWS || runtimeType == hyperscaler.TypeAzure) &&
		runtime.Spec.Shoot.Kubernetes.KubeAPIServer.ACL != nil &&
		len(runtime.Spec.Shoot.Kubernetes.KubeAPIServer.ACL.AllowedCIDRs) > 0
}
```

Every consumer routes through this function:

- `NewExtensionsExtenderForCreate` (`extender.go`) — creates the `acl`
  extension on new Shoots.
- `NewExtensionsExtenderForPatch` (`extender.go`) — creates/updates/removes
  the `acl` extension on existing Shoots.
- `ConfigReloadWatcher` predicate (`cmd/main.go`) — decides whether an ACL
  ConfigMap change should trigger a Runtime reconciliation.

The extension payload itself (`NewApiServerACLExtension` /
`applyAccessControlList`) is provider-agnostic: it emits a generic
`{ action: "ALLOW", cidrs: [...], type: "remote_ip" }` rule.

## Design

### Functional change

Extend the provider check in `AclNeedsToBeEnabled` to include OpenStack:

```go
(runtimeType == hyperscaler.TypeAWS ||
	runtimeType == hyperscaler.TypeAzure ||
	runtimeType == hyperscaler.TypeOpenStack) &&
```

`hyperscaler.TypeOpenStack` already exists in
`pkg/gardener/shoot/hyperscaler/const.go` (value `"openstack"`).

This is the entire functional change. Because all consumers delegate to this
gate, CCEE support propagates automatically to Shoot creation, patching, and
the config-reload watcher.

### What does NOT change

- **API types** — no new fields on the Runtime CR. CCEE reuses the existing
  `KubeAPIServer.ACL.AllowedCIDRs`.
- **Config** — no new operator flags or converter config. The existing
  `api-server-acl-enabled` flag governs CCEE too.
- **NAT IP wiring** — OpenStack NAT gateways are statically addressed; the KCP
  NAT IP handling is unchanged and provider-independent.
- **Extension payload** — `NewApiServerACLExtension` stays generic.

## Testing

### Unit tests

1. `apiServerACL_test.go` — add a dedicated table-driven test for
   `AclNeedsToBeEnabled` covering:
   - OpenStack + flag on + CIDRs present → `true`
   - AWS / Azure → `true` (regression)
   - GCP → `false` (still unsupported; guards against accidental enablement)
   - OpenStack + flag off → `false`
   - OpenStack + empty CIDRs → `false`
   - OpenStack + nil ACL → `false`

2. `extensions_extender_test.go`:
   - Add an OpenStack "should include ACL extension" case to
     `TestNewExtensionsExtenderForCreate`.
   - Add an OpenStack case to `TestNewExtensionsExtenderForPatch`.
   - The existing `"...hyperscaler type is not supported"` case uses
     `TypeGCP` — this stays valid and should remain.

### Manual / E2E (tracked separately by issue AC)

Manual cluster creation in BTP Cockpit and turning on the ACL feature for a
CCEE cluster once the code is merged. Covered by the issue's dedicated AC, not
by this change.

## Documentation

- `docs/e2e-acl-test-plan.md`:
  - Update "ACL only works for **AWS** and **Azure** providers" to include
    OpenStack (CCEE).
  - Fix the stale file reference `pkg/gardener/shoot/extender/kubeserver_acl.go`
    → `pkg/gardener/shoot/extender/extensions/apiServerACL.go`.
- SAP Help external documentation is a tech-writer task (separate AC in the
  issue) and out of scope for this code change.

## Out of Scope

- GCP support — separate, still-blocked issue #1570.
- SAP Help / external documentation updates — tech-writer AC.
- Manual BTP Cockpit verification — separate issue AC.

## Acceptance Criteria (from #1569)

- [x] Ping @kyma-project/gopher to align on turning the ACL feature on for CCEE (SCI)
- [ ] Add ACL support for CCEE (SCI) in the shoot extender
- [ ] Add unit/integration tests
- [ ] Update the documentation
- [ ] Manually test cluster creation in BTP Cockpit and turning on the ACL feature for CCEE (SCI)
