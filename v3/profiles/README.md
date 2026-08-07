# Profiles

A **profile** declares *intent*: an engineering role plus selected
capabilities. Profiles are configuration and state owned by Veilbox
Core — **never RPMs** (see `docs/adr/0001-architecture.md`).

Capability selection happens against the capability catalog:

| Domain | Capabilities |
|---|---|
| cloud | aws, azure, gcp, openstack |
| containers | podman, docker |
| kubernetes | kubernetes, eks, gke, aks, k3s, openshift |
| infrastructure | terraform, opentofu, ansible, pulumi, helm |
| observability | grafana, prometheus, loki, tempo, opentelemetry |

Profile definitions live in this directory (shipped by
`veilbox-core` as packaged data). The Profile Engine merges a base
profile with user-selected capabilities and persists the result as
state under `~/.config/veilbox/`.

The environment stays editable after installation: applying a profile
is a mutation of intent, not a one-shot script.
