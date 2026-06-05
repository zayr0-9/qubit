# Qubit Distribution Guide

Mandatory when working on Qubit release archives, install scripts, GitHub Releases packaging, generated release assets, or installed-runtime layout.

## Current Distribution Strategy

Qubit's first distribution channel is GitHub Releases plus install scripts.

Initial supported targets:

```txt
linux x64    release/qubit-v<version>-linux-x64.tar.gz
windows x64  release/qubit-v<version>-windows-x64.zip
```

The release archive ships the Go CLI together with the Node runtime sidecar files:

```txt
bin/qubit or bin/qubit.exe
VERSION
README.install.txt
dist/
prompts/
package.json
pnpm-lock.yaml
node_modules/
```

Node.js is still a user prerequisite and must be available on `PATH`. The Go CLI discovers the installed app root from the executable location and launches `dist/runtime.js` with Node.

## Important Files

```txt
scripts/package-release.mjs  Builds release archives and checksum files.
scripts/install.sh           Linux/Ubuntu installer.
scripts/install.ps1          Windows installer.
release/                     Generated local release artifacts; ignored by git.
```

Package scripts:

```sh
pnpm run package:release:linux
pnpm run package:release:windows
pnpm run install:dogfood:linux
```

`install:dogfood:linux` rebuilds the Linux release archive and installs it from the local `release/` artifact using `scripts/install.sh`. Use it for fast Ubuntu/Linux dogfooding updates.

## Packaging Rules

- Do not commit generated archives or checksums under `release/`.
- Build `dist/` from TypeScript before packaging; do not edit generated `dist` directly.
- Preserve pnpm symlink layout when copying `node_modules`. Broken or flattened pnpm links can cause runtime errors like `ERR_MODULE_NOT_FOUND` for transitive packages.
- Install scripts must remove the existing versioned install root before extracting a replacement archive, otherwise stale/broken files can survive reinstall.
- Keep the source checkout path out of packaged `node_modules` symlinks; release archives must work after the source repo is deleted.
- Keep project data in the terminal launch cwd: installed Qubit should still write `.qubit/` under the directory where the user runs `qubit`, not under the install directory.
- Keep user-global non-secret settings in the platform config directory, not inside the install directory.

## Install Script Behavior

Install scripts should:

1. Detect supported OS/architecture.
2. Require Node.js on `PATH`.
3. Resolve `latest` through GitHub Releases unless `QUBIT_ARCHIVE_URL` is set.
4. Download the archive and optional `.sha256` file.
5. Verify SHA-256 when checksum is available.
6. Extract to a user-local install root.
7. Point a stable launcher at the extracted version.
8. Print PATH guidance and a stub-mode smoke command.

Useful environment variables:

```txt
QUBIT_REPO          GitHub repo, default zayr0-9/qubit
QUBIT_VERSION       Release tag, default latest
QUBIT_ARCHIVE_URL   Explicit archive URL, useful for local tests
QUBIT_INSTALL_DIR   Install root
QUBIT_BIN_DIR       Directory for the qubit launcher
```

## Validation

After distribution changes, run at least:

```sh
node --check scripts/package-release.mjs
bash -n scripts/install.sh
pnpm run package:release:linux
node ~/.local/share/qubit/current/dist/runtime.js --check
```

For local Ubuntu install testing:

```sh
pnpm run package:release:linux
rm -rf ~/.local/share/qubit/qubit-v0.1.0-linux-x64
rm -f ~/.qubit/runtime-server.lock
QUBIT_ARCHIVE_URL="file://$PWD/release/qubit-v0.1.0-linux-x64.tar.gz" sh scripts/install.sh
cd ~
QUBIT_STUB=1 qubit
```

If launch fails with `connect runtime server: connection refused`, inspect the runtime log in the launch cwd:

```sh
cat .qubit/runtime.log
```

A successful installed runtime check should print:

```txt
runtime check ok
```
