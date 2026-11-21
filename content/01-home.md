---
key: 1
slug: home
title: home
pose: happy
sources: README.md
---
# a headless OS for AI coding agents, on stock Ubuntu Server 26.04

# early alpha. not for production use.
# tested only in QEMU virtual machines, never on real hardware.
# no AI agent can run on it yet.

# what runs today: an unattended, encrypted Ubuntu install with A/B OS generations, plus the first host services of the safety layer.

# the design rests on four rules. only the last one is fully built.
- no credentials in tasks. brokers will hold the keys, and the task never will. [designed]
- no network for agents. the host firewall is built. task containers and brokers are not. [partly built]
- a human signs risky actions, bound to their exact inputs. [designed]
- OS changes fall back on failure. a new generation stays only if it boots healthy. [built]

# box log
[ok] firewall
[ok] auditd
[--] agent runtime: designed

# read this site in your terminal
$ go run github.com/dbabi/agentbox-web@latest
# get the code
$ git clone https://github.com/dbabi/agentbox.git
