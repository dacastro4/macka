#!/usr/bin/env bash
# M0 gate (core-video-stack spec): a clean Debian 12 VM reaches a fully wired
# stack and a second run changes nothing. Creates a throwaway Lima VM, runs the
# contract test in it, then deletes the VM. KEEP=1 keeps the VM for debugging.
#
#   test/gate/run-m0-gate.sh bin/hli-linux-<arch> bin/contract-linux-<arch>.test
set -euo pipefail
cd "$(dirname "$0")/../.."

hli_bin=${1:?usage: run-m0-gate.sh <hli linux binary> <contract test binary>}
ct_bin=${2:?usage: run-m0-gate.sh <hli linux binary> <contract test binary>}
vm="hli-gate-$(date +%s)"
log="test/gate/last-run.log"

cleanup() {
  if [ "${KEEP:-}" = "1" ]; then
    echo "keeping VM $vm (limactl delete -f $vm to remove)"
  else
    limactl delete -f "$vm" >/dev/null 2>&1 || true
  fi
}
trap cleanup EXIT

echo "gate: creating clean Debian 12 VM $vm"
limactl start --name="$vm" --cpus=4 --memory=8 --disk=30 --tty=false template:debian-12 >/dev/null 2>&1
limactl shell "$vm" bash -c "cat /etc/debian_version"

echo "gate: installing Docker Engine"
limactl copy test/gate/install-docker.sh "$vm":/tmp/install-docker.sh
limactl shell "$vm" bash /tmp/install-docker.sh

limactl copy "$hli_bin" "$vm":/tmp/hli
limactl copy "$ct_bin" "$vm":/tmp/contract.test
limactl shell "$vm" chmod +x /tmp/hli /tmp/contract.test

echo "gate: running contract test (two hli apply runs)"
start=$(date +%s)
set +e
limactl shell "$vm" sg docker -c "cd /tmp && HLI_BIN=/tmp/hli ./contract.test -test.v -test.timeout 30m" | tee "$log"
rc=${PIPESTATUS[0]}
set -e
echo "gate: finished in $(( $(date +%s) - start ))s, exit $rc (log: $log)"
exit "$rc"
