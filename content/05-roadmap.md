---
key: 5
slug: roadmap
title: roadmap
pose: thinking
sources: docs/wiki/Roadmap.md, docs/design/PLAN.md
---
# early alpha. the milestones, in order. each ends with a test gate that must pass before the next starts.

1. installer and safe OS updates [built]
# encrypted install, recovery key, A/B generations, signed releases. passed in QEMU VMs; no real PC yet.
2. firewall, audit log, root daemon [built]
# passed in VMs, in none mode on arm64 only.
3. operation registry and request gate [in progress]
4. agent accounts and a sealed container per task [next]
5. brokers that hold the keys: proxy, keys, git [planned]
6. signed approvals [planned]
7. apps and OS updates as new generations [planned]
8. agent bus and shared memory [planned]
9. benchmark and the first signed release [planned]

# what has not run
- [not run] the install and os apply checks on a real x86 PC.
- [not run] an amd64 UEFI install, anywhere.
- [not run] the firewall and audit log gates in tpm2, tpm2+tang and on amd64. they ran in none mode on arm64 UEFI only.
- [not run] power cuts during os apply in none and tpm2+tang modes.

# known gaps
- no inbound filtering. not on the roadmap yet. keep the box on a network you trust.
- no automatic OS updates. milestone 7.
- no signed approvals. milestone 6.
- no way to update agentbox on an installed box. a new release means a reinstall.
- [Roadmap](https://github.com/dbabi/agentbox/blob/main/docs/wiki/Roadmap.md)
