#!/usr/bin/env sh
set -eu

REPO="${QUBIT_REPO:-zayr0-9/qubit}"
VERSION="${QUBIT_VERSION:-latest}"
INSTALL_DIR="${QUBIT_INSTALL_DIR:-$HOME/.local/share/qubit}"
BIN_DIR="${QUBIT_BIN_DIR:-$HOME/.local/bin}"
ARCHIVE_URL="${QUBIT_ARCHIVE_URL:-}"

os="$(uname -s | tr '[:upper:]' '[:lower:]')"
arch="$(uname -m)"

if [ "$os" != "linux" ]; then
  echo "Qubit install.sh currently supports Linux only." >&2
  exit 1
fi
case "$arch" in
  x86_64|amd64) arch="x64" ;;
  *) echo "Unsupported architecture: $arch" >&2; exit 1 ;;
esac

need() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "Missing required command: $1" >&2
    exit 1
  fi
}

need curl
need tar
need node

if [ -z "$ARCHIVE_URL" ]; then
  if [ "$VERSION" = "latest" ]; then
    VERSION="$(curl -fsSL "https://api.github.com/repos/$REPO/releases/latest" | sed -n 's/.*"tag_name"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' | head -n 1)"
    if [ -z "$VERSION" ]; then
      echo "Could not resolve latest release for $REPO" >&2
      exit 1
    fi
  fi
  version_no_v="$(printf '%s' "$VERSION" | sed 's/^v//')"
  asset="qubit-v${version_no_v}-linux-${arch}.tar.gz"
  ARCHIVE_URL="https://github.com/$REPO/releases/download/$VERSION/$asset"
else
  asset="$(basename "$ARCHIVE_URL")"
fi

tmp="$(mktemp -d)"
cleanup() { rm -rf "$tmp"; }
trap cleanup EXIT INT TERM

archive="$tmp/$asset"
echo "Downloading $ARCHIVE_URL"
curl -fL "$ARCHIVE_URL" -o "$archive"

checksum_url="${ARCHIVE_URL}.sha256"
if curl -fsL "$checksum_url" -o "$archive.sha256"; then
  if command -v sha256sum >/dev/null 2>&1; then
    (cd "$tmp" && sha256sum -c "$(basename "$archive.sha256")")
  else
    echo "sha256sum not found; skipping checksum verification" >&2
  fi
else
  echo "No checksum found at $checksum_url; skipping checksum verification" >&2
fi

mkdir -p "$INSTALL_DIR" "$BIN_DIR"
extracted="$(tar -tzf "$archive" | head -n 1 | cut -d/ -f1)"
install_root="$INSTALL_DIR/$extracted"
rm -rf "$install_root"
tar -xzf "$archive" -C "$INSTALL_DIR"

if [ ! -x "$install_root/bin/qubit" ]; then
  echo "Archive did not contain executable bin/qubit" >&2
  exit 1
fi

ln -sfn "$install_root" "$INSTALL_DIR/current"
cat > "$BIN_DIR/qubit" <<EOF
#!/usr/bin/env sh
exec "$INSTALL_DIR/current/bin/qubit" "\$@"
EOF
chmod +x "$BIN_DIR/qubit"

echo "Qubit installed to $install_root"
echo "Launcher written to $BIN_DIR/qubit"
case ":$PATH:" in
  *":$BIN_DIR:"*) ;;
  *) echo "Add $BIN_DIR to PATH, for example: export PATH=\"$BIN_DIR:\$PATH\"" ;;
esac
echo "Try: QUBIT_STUB=1 qubit"
