---
key: 3
slug: how
title: how
pose: thinking
sources: docs/wiki/Architecture.md, docs/wiki/Generations-and-Updates.md, docs/wiki/Release-Pipeline.md, docs/design/ARCHITECTURE.md
---
# how agentbox works, in five parts. four are built. the agent runtime is designed.

# 1. unattended encrypted install from one seed [built]
# agentbox seed checks every input, then writes the autoinstall seed.
# the install stops before any disk write unless the serial matches exactly one disk.
# disk encryption: tpm2+tang (the default), tpm2, or none. a recovery key unlocks it offline.
- [Encryption and recovery](https://github.com/dbabi/agentbox/blob/main/docs/wiki/Encryption-and-Recovery.md)

# 2. A/B generations: build offline, boot once, keep only if healthy [built]
# each OS version is a btrfs subvolume with its own /boot directory.
# os plan and os apply build a package change in a container with no network.
# the new generation boots once. a health check commits it. anything else falls back.
- [Generations and updates](https://github.com/dbabi/agentbox/blob/main/docs/wiki/Generations-and-Updates.md)

# 3. signed reproducible releases [built]
# make release builds a tarball and a minisign signature.
# a Linux CI build and a Mac rebuild are byte-identical before signing.
# first boot checks a pinned SHA-256, then the signature. there is no official release yet.
$ make release VERSION=0.0.1 MINISIGN_KEY="$HOME/agentbox-keys/minisign.key"
- [Release pipeline](https://github.com/dbabi/agentbox/blob/main/docs/wiki/Release-Pipeline.md)

# 4. host services: firewall, auditd hash chain, rootd resolver [built]
# the firewall limits each agentbox service user to its own destinations.
# auditd records each operation durably before it runs, in a hash-chained log.
# rootd resolves names for the brokers and fills their firewall sets.
- [Architecture](https://github.com/dbabi/agentbox/blob/main/docs/wiki/Architecture.md)

# 5. the agent runtime: task containers, brokers, approvals [designed]
# one disposable container per task, with no network and a fresh HOME.
# brokers hold the keys and act for the task. a human signs risky operations.
- [Design notes: ARCHITECTURE.md](https://github.com/dbabi/agentbox/blob/main/docs/design/ARCHITECTURE.md)

```
 laptop: signs approvals (designed)
        |
 +------v--------------------------------------+
 | agentbox host: stock Ubuntu Server 26.04    |
 |  task container --sockets--> brokers -----------> internet
 |  no network, no keys         proxy keys git |
 |  (designed)                  (designed)     |
 |  ------------------------------------------ |
 |  built: firewall  auditd  rootd (skeleton)  |
 |         quotas  generations  releases LUKS2 |
 +---------------------------------------------+
```
# the same picture in words:
- a task container has no network and no keys. [designed]
- it talks to brokers over unix sockets. only brokers reach the internet. [designed]
- risky operations wait for a signature from your laptop. [designed]
- under them, the built layer: firewall, auditd, the rootd skeleton, disk quotas, OS generations, signed releases and LUKS2 disk encryption. [built]
