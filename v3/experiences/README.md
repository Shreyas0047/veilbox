# Experiences

An **experience** is an installable capability: a complete, coherent
set of Fedora packages delivered as an RPM meta-package
(`veilbox-experience-<name>`), never as bare packages. Veilbox
installs and removes experiences through DNF by package name, exactly
like any other repository package.

## Current catalog (v3 prototype)

| Experience | Meta-package | Packages | Status |
|---|---|---|---|
| base-ops | veilbox-experience-base-ops | git, vim-enhanced, curl, strace | available |
| networking-tools | veilbox-experience-networking-tools | bind-utils, traceroute, nmap-ncat, iproute, tcpdump | available |
| terminal-ops | veilbox-experience-terminal-ops | tmux, ripgrep, htop | available |
| observability-cli | veilbox-experience-observability-cli | sysstat, iotop, jq | available |

Status meanings: `planned` (declared, not yet packaged), `available`
(installable), `installed` (present in the RPM database).

Removal semantics: DNF is the transaction authority. Removing an
experience meta-RPM removes packages that were introduced solely as
its dependencies and are needed by nothing else; packages the user
already had before the experience was installed are never removed.

Experience definitions (`experiences/<name>.yaml`) declare the
meta-package (`rpm:`) and the concrete packages (informational; the
RPM `Requires:` are authoritative).

Desktop experiences (compositor + complete shell + configuration)
arrive in a later milestone.

See `docs/adr/0002-delivery-model.md` for the RPM delivery model and
`docs/day3-intent-engine.md` for the current catalog.
