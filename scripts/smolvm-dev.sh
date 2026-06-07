#!/usr/bin/env bash
set -euo pipefail

VM_NAME="${QUBIT_SMOLVM_NAME:-qubit-dev}"
IMAGE="${QUBIT_SMOLVM_IMAGE:-node:22-bookworm}"
PROJECT_DIR="${QUBIT_PROJECT_DIR:-$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)}"
COPY_MODE="${QUBIT_SMOLVM_COPY:-0}"
if [[ "$COPY_MODE" == "1" ]]; then
  WORKSPACE="${QUBIT_SMOLVM_WORKSPACE:-/workspace/qubit}"
else
  WORKSPACE="${QUBIT_SMOLVM_WORKSPACE:-/workspace}"
fi
GO_VERSION="${QUBIT_GO_VERSION:-1.26.3}"

usage() {
  cat <<'EOF'
Usage: scripts/smolvm-dev.sh <command> [args]

Create and use a persistent smolvm VM for Qubit development/testing.
Default image: node:22-bookworm
Default VM:    qubit-dev
Default mount: current Qubit repo -> /workspace
Copy mode:     set QUBIT_SMOLVM_COPY=1 to copy repo into the VM instead of mounting it

Commands:
  create        Create the VM if missing and install OS dependencies
  start         Start the VM
  sync          Copy host repo into the VM (copy mode only)
  setup         Install/refresh Qubit dependencies inside the VM
  build         Build runtime and CLI inside the VM
  check         Run runtime checks, Go tests, vet, and native SQLite smoke
  smoke         Run a non-interactive stub/runtime smoke check
  shell         Open an interactive shell in the VM
  chat          Run Qubit TUI in stub mode inside the VM
  all           create + start + setup + build + check + smoke
  stop          Stop the VM
  delete        Delete the VM
  status        Show VM status
  exec -- CMD   Run arbitrary command inside /workspace

Environment overrides:
  QUBIT_SMOLVM_NAME       VM name, default qubit-dev
  QUBIT_SMOLVM_IMAGE      OCI image, default node:22-bookworm
  QUBIT_PROJECT_DIR       Host repo path, default script parent
  QUBIT_SMOLVM_WORKSPACE  Guest workspace path, default /workspace or /workspace/qubit in copy mode
  QUBIT_SMOLVM_COPY       Set to 1 for safer copy-in mode instead of host bind mount
  QUBIT_GO_VERSION        Go version to install, default 1.26.3

Examples:
  scripts/smolvm-dev.sh all
  scripts/smolvm-dev.sh shell
  scripts/smolvm-dev.sh chat
  scripts/smolvm-dev.sh exec -- pnpm run check:runtime
  QUBIT_SMOLVM_IMAGE=ubuntu:24.04 scripts/smolvm-dev.sh all
  QUBIT_SMOLVM_COPY=1 scripts/smolvm-dev.sh all
EOF
}

need_smolvm() {
  if ! command -v smolvm >/dev/null 2>&1; then
    echo "Missing smolvm on PATH. Install smolvm first." >&2
    exit 1
  fi
}

vm_exists() {
  smolvm machine status --name "$VM_NAME" >/dev/null 2>&1
}

run_in_vm() {
  smolvm machine exec --name "$VM_NAME" --workdir "$WORKSPACE" -e "QUBIT_GO_VERSION=$GO_VERSION" -- "$@"
}

run_shell_script() {
  local script="$1"
  run_in_vm bash -lc "$script"
}

create_vm() {
  need_smolvm
  if vm_exists; then
    echo "VM '$VM_NAME' already exists."
  else
    echo "Creating VM '$VM_NAME' from $IMAGE..."
    if [[ "$COPY_MODE" == "1" ]]; then
      smolvm machine create --net --image "$IMAGE" "$VM_NAME"
    else
      smolvm machine create --net --image "$IMAGE" -v "$PROJECT_DIR:$WORKSPACE" "$VM_NAME"
    fi
  fi
  start_vm
  install_os_deps
  sync_repo_if_copy_mode
}

start_vm() {
  need_smolvm
  smolvm machine start --name "$VM_NAME"
}

install_os_deps() {
  echo "Installing OS dependencies in '$VM_NAME'..."
  run_shell_script '
set -euo pipefail
export DEBIAN_FRONTEND=noninteractive
apt-get update
apt-get install -y --no-install-recommends \
  bash \
  ca-certificates \
  curl \
  git \
  ripgrep \
  build-essential \
  pkg-config \
  python3 \
  make \
  tar \
  gzip \
  libsecret-1-dev \
  dbus-x11 \
  xdg-utils \
  libnotify-bin
if ! command -v node >/dev/null 2>&1 || ! command -v npm >/dev/null 2>&1; then
  apt-get install -y --no-install-recommends nodejs npm
fi
case "$(uname -m)" in
  x86_64|amd64) go_arch=amd64 ;;
  aarch64|arm64) go_arch=arm64 ;;
  *) echo "unsupported Go install arch: $(uname -m)" >&2; exit 1 ;;
esac
curl -fsSL -o /tmp/go.tgz "https://go.dev/dl/go${QUBIT_GO_VERSION:-1.26.3}.linux-${go_arch}.tar.gz"
rm -rf /usr/local/go
tar -C /usr/local -xzf /tmp/go.tgz
ln -sf /usr/local/go/bin/go /usr/local/bin/go
ln -sf /usr/local/go/bin/gofmt /usr/local/bin/gofmt
npm install -g pnpm@10.33.0
pnpm --version
node --version
go version
'
}

sync_repo_if_copy_mode() {
  if [[ "$COPY_MODE" != "1" ]]; then
    return 0
  fi
  start_vm
  echo "Copying repo into '$VM_NAME:$WORKSPACE'..."

  local archive
  archive="$(mktemp -t qubit-smolvm-sync.XXXXXX.tar.gz)"

  tar -czf "$archive" \
    --exclude .git \
    --exclude .qubit \
    --exclude node_modules \
    --exclude dist \
    --exclude bin \
    --exclude release \
    -C "$PROJECT_DIR" .

  smolvm machine exec --name "$VM_NAME" -- mkdir -p "$(dirname "$WORKSPACE")" /tmp/qubit-sync
  smolvm machine cp "$archive" "$VM_NAME:/tmp/qubit-sync/source.tar.gz"
  smolvm machine exec --name "$VM_NAME" -- rm -rf "$WORKSPACE"
  smolvm machine exec --name "$VM_NAME" -- mkdir -p "$WORKSPACE"
  smolvm machine exec --name "$VM_NAME" -- tar -xzf /tmp/qubit-sync/source.tar.gz -C "$WORKSPACE"
  smolvm machine exec --name "$VM_NAME" -- rm -f /tmp/qubit-sync/source.tar.gz
  rm -f "$archive"
}

setup_qubit() {
  start_vm
  sync_repo_if_copy_mode
  echo "Installing Qubit Node dependencies..."
  run_shell_script '
set -euo pipefail
CI=true pnpm install
pnpm rebuild better-sqlite3 keytar || true
'
}

build_qubit() {
  start_vm
  echo "Building Qubit..."
  run_shell_script '
set -euo pipefail
pnpm run build:runtime
go build -o bin/qubit .
'
}

check_qubit() {
  start_vm
  echo "Running Qubit checks..."
  run_shell_script '
set -euo pipefail
pnpm run check:runtime
go test ./...
go vet ./...
go build -o bin/qubit .
node -e "import Database from '\''better-sqlite3'\''; const db = new Database('\'':memory:'\''); db.exec('\''select 1'\''); db.close(); console.log('\''better-sqlite3 ok'\'')"
'
}

smoke_qubit() {
  start_vm
  echo "Running Qubit non-interactive smoke checks..."
  run_shell_script '
set -euo pipefail
QUBIT_STUB=1 node dist/runtime.js --check
QUBIT_STUB=1 ./bin/qubit --help >/tmp/qubit-help.txt || true
head -40 /tmp/qubit-help.txt || true
'
}

shell_vm() {
  start_vm
  smolvm machine exec --name "$VM_NAME" --workdir "$WORKSPACE" -it -- bash
}

chat_vm() {
  start_vm
  smolvm machine exec --name "$VM_NAME" --workdir "$WORKSPACE" -it -- bash -lc 'QUBIT_STUB=1 ./bin/qubit'
}

status_vm() {
  need_smolvm
  smolvm machine status --name "$VM_NAME"
}

stop_vm() {
  need_smolvm
  smolvm machine stop --name "$VM_NAME"
}

delete_vm() {
  need_smolvm
  smolvm machine delete "$VM_NAME"
}

exec_vm() {
  start_vm
  if [[ "${1:-}" == "--" ]]; then shift; fi
  if [[ $# -eq 0 ]]; then
    echo "exec requires a command" >&2
    exit 1
  fi
  run_in_vm "$@"
}

cmd="${1:-}"
if [[ $# -gt 0 ]]; then shift; fi

case "$cmd" in
  create) create_vm ;;
  sync) start_vm; sync_repo_if_copy_mode ;;
  start) start_vm ;;
  setup) setup_qubit ;;
  build) build_qubit ;;
  check) check_qubit ;;
  smoke) smoke_qubit ;;
  shell) shell_vm ;;
  chat) chat_vm ;;
  all) create_vm; setup_qubit; build_qubit; check_qubit; smoke_qubit ;;
  status) status_vm ;;
  stop) stop_vm ;;
  delete|rm) delete_vm ;;
  exec) exec_vm "$@" ;;
  -h|--help|help|"") usage ;;
  *) echo "Unknown command: $cmd" >&2; usage >&2; exit 1 ;;
esac
