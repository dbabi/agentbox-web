---
key: 4
slug: security
title: security
pose: guarding
sources: docs/wiki/Security-Model.md, docs/wiki/Encryption-and-Recovery.md, docs/wiki/Roadmap.md, SECURITY.md
---
# the threat: prompt injection from untrusted text.
# web pages, issues and package READMEs can carry instructions.
# a hijacked task should find nothing to steal and no way to send it out.

# enforced today
- host firewall: each service user reaches only its own destinations. fwcheck restores an altered table. [built]
- hash-chained audit log. agentbox audit verify checks the chain. with auditd down, rootd runs nothing. [built]
- rootd resolver: refuses private and special-purpose answers for the brokers. [built]
- disk encryption with LUKS2, and an offline recovery key. [built]

# designed, not built
- brokers hold the keys. no credential exists inside a task. [designed]
- human-signed operations, bound to their exact inputs. [designed]

# known gaps
- no inbound filtering. SSH and any other listener are open to your LAN. do not give the box a public IP.
- no automatic security updates. an admin runs os plan and os apply by hand.
- in tpm2 mode, a thief with the whole machine can unlock the disk. use tpm2+tang.
- [Security model](https://github.com/dbabi/agentbox/blob/main/docs/wiki/Security-Model.md)
- [Encryption and recovery](https://github.com/dbabi/agentbox/blob/main/docs/wiki/Encryption-and-Recovery.md)

# this site
- no login, no cookies, no analytics, no third-party requests.
- the web page keeps one localStorage flag, for the keys toggle. nothing else.
- the SSH version is read-only and listens on 127.0.0.1 by default.
- it has no shell, no port or agent forwarding and no file transfer. the only commands are page names.

# found a vulnerability? report it privately. do not open a public issue.
- [SECURITY.md](https://github.com/dbabi/agentbox/blob/main/SECURITY.md)
