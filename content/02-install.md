---
key: 2
slug: install
title: install
pose: happy
sources: docs/wiki/Getting-Started.md, docs/wiki/Installing-on-Real-Hardware.md, docs/wiki/Testing-and-Gates.md
---
# try agentbox in a QEMU VM on an Apple Silicon Mac.
# you need about 12 GB free for .vm/, 4 CPU cores and 4 GiB of RAM for the VM, and internet.

1. install the tools. the encrypted gates also need swtpm and podman.
$ brew install go qemu xorriso minisign uv
# also: xcode-select --install for make and ar, and a sha256sum on PATH.
2. get the code. run every later command from the top of the checkout.
$ git clone https://github.com/dbabi/agentbox.git && cd agentbox
3. get the pinned Ubuntu ISO, 3.1 GB.
$ mkdir -p .vm && curl -fL -o .vm/ubuntu-26.04.1-live-server-arm64.iso https://cdimage.ubuntu.com/releases/26.04.1/release/ubuntu-26.04.1-live-server-arm64.iso
# on a 404, get the same file from https://old-releases.ubuntu.com/releases/26.04.1/
4. check it. this prints ubuntu-26.04.1-live-server-arm64.iso: OK
$ (cd .vm && grep live-server-arm64.iso ../hack/SHA256SUMS | shasum -a 256 -c)
5. build the CLI and run the unit tests.
$ go build -o bin/agentbox ./cmd/agentbox && go vet ./... && go test ./...
6. optional: render a test seed with no VM, in about a minute. see [Getting started, step 5](https://github.com/dbabi/agentbox/blob/main/docs/wiki/Getting-Started.md#step-5-render-a-test-seed-dry-run-no-vm).
7. run the install gate. keep the Mac on power, with the lid open.
# gate: 9-17 min
$ caffeinate -i hack/gate.sh none
8. read the results. a pass prints: gate none: all checks passed. these print nothing, then 2.
$ grep '^FAIL' .vm/gate-none.txt
$ grep -c '^fails=0' .vm/gate-none.txt
9. clean up. this deletes the VM disk and keeps the ISO.
$ hack/vm.sh reset

# next: build, boot and commit new OS generations in an encrypted VM. needs swtpm.
# apply-gate: about 30 min
$ caffeinate -i hack/apply-gate.sh tpm2 install setup cycles

> CAUTION: no real machine has run agentbox. the install erases the disk you name.
# a real PC install uses two USB sticks and a disk serial. amd64 UEFI and real hardware have not run.
- [Installing on real hardware](https://github.com/dbabi/agentbox/blob/main/docs/wiki/Installing-on-Real-Hardware.md)
- [Testing and gates](https://github.com/dbabi/agentbox/blob/main/docs/wiki/Testing-and-Gates.md)
