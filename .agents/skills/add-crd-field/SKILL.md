---
name: add-crd-field
description: Add a configurable field to the SolrCloud and/or SolrPrometheusExporter CRD - the Go field and its kubebuilder markers, the plumbing into the generated Kubernetes objects, the Helm value and its mapping, the regenerated CRDs and deepcopy code, a controller test, and the Helm changelog entry. Use this for the repeated "make X configurable" change, so the steps that pass CI while doing nothing are not skipped.
when_to_use: The user asks to make something configurable, expose a Kubernetes or Solr setting through the CRD, add a field or option to SolrCloud or SolrPrometheusExporter, or surface an existing CRD field as a Helm value.
---

# Add a CRD field

Adding a field is mechanical, but three of its steps pass CI while doing nothing: a missing Helm
value mapping, a missing README table row, and a missing changelog entry. Read `AGENTS.md` first —
where it and this skill disagree, the repo wins.

## Scope it first

Ask these three before touching a file, since each answer changes the file set. Do not ask as one
free-text question.

1. **Which resources?** SolrCloud, SolrPrometheusExporter, or both. Both means
   `api/v1beta1/common_types.go`, `prometheus_exporter_util.go` and a second controller test.
2. **Reachable from the Helm chart?** If yes, `values.yaml`, the mapping template and the README
   table are in scope. A CRD-only field skips all three.
3. **How is it validated?** The assertion the new controller test makes.

Stop and re-scope if the answer implies controller logic beyond plumbing a value into a generated
object. That is a feature, not a field.

## Branch from upstream main

```bash
git remote -v | awk '/apache\/solr-operator(\.git)? \(fetch\)/ {print $1}'
git fetch <remote> main
git switch -c <issue-or-topic> <remote>/main
```

The remote is `upstream` in some clones, `apache` or `origin` in others; ask rather than guess if
the first command prints nothing. Stop and ask if tracked files are modified. Never commit to
`main`.

## The file set

**Hand-written**

| File | When |
|---|---|
| `api/v1beta1/common_types.go` | The field belongs to `PodOptions` or another type both CRDs share. |
| `api/v1beta1/solrcloud_types.go` | A top-level SolrCloud spec field. |
| `controllers/util/common.go` | Plumbing a shared pod field into the PodSpec, and its update-detection branch. |
| `controllers/util/solr_util.go` | Plumbing into the SolrCloud StatefulSet. |
| `controllers/util/prometheus_exporter_util.go` | Only when the field applies to the exporter too. |
| `controllers/solrcloud_controller_test.go` | Always. Add `solrprometheusexporter_controller_test.go` when the field applies to both. |
| `helm/solr/values.yaml` | The value, with its comment. |
| `helm/solr/templates/_custom_option_helpers.tpl` | **Maps a `podOptions` value into the CRD. The step contributors miss** — without it the value exists and does nothing. A top-level spec field is mapped in `helm/solr/templates/solrcloud.yaml` instead. |
| `helm/solr/README.md` | The values table. Hand-maintained, not generated from `values.yaml`. |
| `helm/solr-operator/Chart.yaml` | The `artifacthub.io/changes` entry. Note the asymmetry: the Helm value lives in the *solr* chart, but a CRD change is announced in the *operator* chart. A change that only adds a chart value, with no CRD change, is announced in `helm/solr/Chart.yaml` instead. |
| `docs/modules/solr-cloud/pages/custom-kube-options.adoc` | A `customSolrKubeOptions` field that needs more than the values table. Keep it short. |
| `docs/modules/solr-cloud/pages/solr-cloud-crd.adoc` | Same, for a top-level SolrCloud spec field. |

**Generated — never hand-edit**

- `api/v1beta1/zz_generated.deepcopy.go`
- `config/crd/bases/solr.apache.org_solrclouds.yaml` and
  `config/crd/bases/solr.apache.org_solrprometheusexporters.yaml`
- `helm/solr-operator/crds/crds.yaml` (a concatenation of the above)

## Conventions

- Optional booleans are `*bool`, so unset is distinguishable from `false`. Tests build them with
  `newBoolPtr`.
- Mark every new field `+optional`, and give it a `+kubebuilder:default=` only when the default is
  part of the API rather than controller behaviour.
- The field's Go doc comment becomes the CRD description that users read through
  `kubectl explain`. Write it for them, and state any version requirement there.
- Reuse the Kubernetes well-known constants (`corev1.LabelTopologyZone` and friends) rather than
  retyping string literals.

## Regenerate

Every `make` target refuses to run unless the running Go matches the `go` directive in `go.mod`.
Prefix make with that toolchain rather than downgrading Go. `GOTOOLCHAIN` needs a full `goX.Y.Z`,
so add the patch version to the two-part number the Makefile's error message prints:

```bash
GOTOOLCHAIN=go1.27.0 make generate manifests
```

Use `generate manifests`, not `make prepare`: `prepare` also regenerates the dependency license
list and tidies the go modules, which belong to the `dependency-upgrade` skill, not here.
`make check-manifests` diffs `config/` and `helm/` recursively, so a partial regeneration fails CI.

## Verify

```bash
GOTOOLCHAIN=... make check-manifests check-generated check-format vet check-helm
GOTOOLCHAIN=... make unit-tests
```

Run the tests through `make unit-tests`, never a bare `go test`, which misses both variables the
target sets: `KUBEBUILDER_ASSETS` points at the envtest binaries, and
`GINKGO_EDITOR_INTEGRATION=true` suppresses Ginkgo's programmatic-focus failure. The specs are
written with `FDescribe`/`FContext`/`FIt` throughout, so without it `go test` reports every spec
passing and still exits non-zero with `PASS | FOCUSED`.

Render the chart both ways to prove the mapping works and the default is unchanged:

```bash
helm template test helm/solr --set <value>=<non-default> | grep -A2 <fieldName>
helm template test helm/solr | grep -c <fieldName>
```

A field addition does not need the e2e suite.

## Changelog

One entry in `helm/solr-operator/Chart.yaml` under `artifacthub.io/changes`, `kind: added`, with a
link to the issue. Say what the field does for the user, not how it is implemented. A field that
makes the operator touch a new Kubernetes resource type also changes the generated RBAC, which is
user-visible and needs its own entry.

## Out of scope

- A new custom resource, or a new API version (`v1beta1` -> `v1`). Conversion and storage-version work.
- Renaming, removing or retyping an existing field. CRD fields are public API, so those need a
  deprecation cycle and the `deprecated`/`removed` changelog kinds.
- Changing an existing field's default. That is a behaviour change for existing users.
- Controller behaviour beyond plumbing a value into a generated object.
