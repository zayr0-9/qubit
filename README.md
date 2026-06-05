# Qubit

Qubit is an extra-lightweight CLI harness for coding agents: a small terminal UI, a Node runtime sidecar, fork visualisation, and a custom tool ecosystem. The goal is to be as simple and efficient as possible while still giving agents the practical shell, file, planning, and session primitives they need.

## What it is

- **Tiny terminal-first harness**: a Go Bubble Tea CLI with a focused chat interface and keyboard-driven workflow.
- **Fork-aware sessions**: branch a conversation, edit or reroll from earlier user messages, and inspect lineage with a visual fork tree.
- **Custom tool ecosystem**: model-callable filesystem, search, shell, todo, plan, multi-call, subagent, and project Markdown tools with runtime permission handling.
- **Provider-flexible runtime**: a TypeScript sidecar connects the TUI to `hyper-router`, model providers, SQLite transcript storage, key management, and JSON-lines transport.
- **Simple process boundary**: Go owns terminal UX; Node owns providers, storage, tools, and session protocol; they communicate with one JSON object per line.

## Project shape

```txt
Qubit CLI        Go Bubble Tea terminal UI: rendering, input, commands, session/fork views
Qubit runtime    Node/TypeScript sidecar: providers, storage, tools, JSON-lines server
hyper-router     Pure TypeScript SDK dependency used by the runtime
```

Important paths:

```txt
main.go                         CLI entrypoint
internal/tui/                   Bubble Tea app, views, input, commands, runtime client
runtime.ts                      Node runtime sidecar source
runtime/                        Runtime support modules
prompts/                        Editable plan/edit/subagent prompt addenda
tools/                          Model-callable tool implementations
utils/                          Shared runtime/tool helpers
.qubit/                         Project-local sessions, indexes, logs, todos, plans
bin/qubit                       Built Linux/macOS executable
bin/qubit.exe                   Built Windows executable
```

## Features

### Chat and sessions

- Persistent chat transcripts through SQLite-backed `hyper-router` storage.
- Session picker via `/sessions`.
- Session creation with `/new [title]`.
- Session rename/favourite flows.
- Singleton runtime attachment per project `.qubit` directory, so multiple TUIs can reuse the same runtime server.

### Fork visualisation

- `/fork` branches from the current point.
- `/fork <message-number>` edits/rerolls from a prior user message.
- `/tree` opens a keyboard-first horizontal fork map.
- Left/right navigates parent/child lineage; Up/Down jumps parallel branches; `j`/`k` preserve linear navigation.

### Tools and permissions

Qubit includes a local custom tool ecosystem for agentic work:

- `readFile`, `readFiles`, `glob`, `ripgrep`
- `bash`, `powershell`
- `createFile`, `editFile`, `multiEdit`, `deleteFile`
- `todoMd`, `planMd`
- `subagent`, `multiCall`

Permission modes are controlled in the UI:

```txt
/permission plan       Ask before gated tools; planning tools stay lightweight
/permission edit       Auto-allow gated tools for implementation work
/permission allow-all  Auto-allow gated tools while keeping the plan prompt
```

Tool filesystem access is restricted to the launch cwd by default. Use `/cwd-remove-block` to allow permitted tools outside the cwd for the current TUI session, and `/cwd-enable-block` to restore the block.

### Providers and keys

Provider selection and model selection are UI flows:

```txt
/providers      Choose or persist the active provider
/models         Choose or persist the active model
/keys           Manage provider API keys through the OS keychain
/subagents      Choose the default provider/model for hidden subagent runs
```

Supported provider families in the runtime include GLM/Z.ai, HyperRouter, OpenAI, Amazon Bedrock, OpenRouter, and ChatGPT Codex OAuth.

Environment variables can also configure providers for automation:

```powershell
$env:QUBIT_PROVIDER = "glm"
$env:ZAI_API_KEY = "your-key"
$env:GLM_MODEL = "glm-5.1"
```

```sh
export QUBIT_PROVIDER=glm
export ZAI_API_KEY=your-key
export GLM_MODEL=glm-5.1
```

Useful development mode:

```powershell
$env:QUBIT_STUB = "1"
```

```sh
export QUBIT_STUB=1
```

## Install

The first distribution path is GitHub Releases with platform archives and install scripts. Linux x64 and Windows x64 are the initial targets. The archive includes the Go CLI plus the Node runtime files (`dist/`, `prompts/`, `package.json`, `pnpm-lock.yaml`, and `node_modules/`). Node.js must still be installed and available on `PATH`.

Linux/Ubuntu install from a release:

```sh
curl -fsSL https://raw.githubusercontent.com/zayr0-9/qubit/main/scripts/install.sh | sh
```

For a local Ubuntu install test before publishing a release, build an archive and point the installer at it:

```sh
pnpm run package:release:linux
QUBIT_ARCHIVE_URL="file://$PWD/release/qubit-v0.1.0-linux-x64.tar.gz" sh scripts/install.sh
QUBIT_STUB=1 qubit
```

Windows install from a release:

```powershell
iwr https://raw.githubusercontent.com/zayr0-9/qubit/main/scripts/install.ps1 -UseB | iex
```

Useful installer environment variables:

```txt
QUBIT_REPO          GitHub repo, default zayr0-9/qubit
QUBIT_VERSION       Release tag, default latest
QUBIT_ARCHIVE_URL   Explicit archive URL, useful for local tests
QUBIT_INSTALL_DIR   Install root
QUBIT_BIN_DIR       Directory for the qubit launcher
```

Build release archives:

```sh
pnpm run package:release:linux
pnpm run package:release:windows
```

For local Ubuntu/Linux dogfooding, rebuild and reinstall the local release in one command:

```sh
pnpm run install:dogfood:linux
```

## Build and run

Prerequisites:

- Go matching `go.mod`
- Node.js and `pnpm`
- Native dependency support for `better-sqlite3` and `keytar`
- Linux local development also needs common build/search tools such as `git`, `ripgrep`, `gcc`, `make`, `pkg-config`, and Secret Service/libsecret development packages for `keytar`

Debian/Ubuntu-style package baseline:

```sh
sudo apt install git ripgrep build-essential pkg-config libsecret-1-dev dbus-x11 xdg-utils
```

Install dependencies:

```powershell
pnpm install
```

```sh
pnpm install
```

Build everything:

```powershell
pnpm run build
```

```sh
pnpm run build
```

Run from source:

```powershell
pnpm run chat
```

```sh
pnpm run chat
```

Run the built binary:

```powershell
.\bin\qubit.exe
```

```sh
./bin/qubit
```

Stub-mode smoke test:

```powershell
$env:QUBIT_STUB = "1"
.\bin\qubit.exe
```

```sh
QUBIT_STUB=1 ./bin/qubit
```

## Validation

Common checks:

```powershell
pnpm run check:runtime
go test ./...
go vet ./...
go build -o bin\qubit.exe .
```

```sh
pnpm run check:runtime
go test ./...
go vet ./...
go build -o bin/qubit .
```

Runtime native SQLite smoke test:

```powershell
node -e "import Database from 'better-sqlite3'; const db = new Database(':memory:'); db.exec('select 1'); db.close(); console.log('ok')"
```

Linux keytar smoke test, after Secret Service is available:

```sh
node -e "import('keytar').then(async mod => { const keytar = mod.default ?? mod; const account = 'qubit-smoke-' + Date.now(); await keytar.setPassword('Qubit Test', account, 'secret'); const got = await keytar.getPassword('Qubit Test', account); await keytar.deletePassword('Qubit Test', account); if (got !== 'secret') throw new Error('keytar round trip failed'); console.log('keytar round trip ok'); })"
```

## Design principles

- Keep the harness light, boring, and fast.
- Keep Go terminal UX separate from Node runtime/provider/tool concerns.
- Keep `hyper-router` pure; Qubit-specific UI and runtime behavior stays in Qubit.
- Prefer additive JSON-lines protocol messages over overloaded shapes.
- Prefer keyboard-first flows and calm terminal rendering.
- Store secrets only in the OS keychain; never in project JSON files or logs.

## Useful slash commands

```txt
/new [title]                 New chat session
/sessions                    Session picker
/fork [title|message-number] Fork or edit/reroll from a prior user message
/tree                        Fork tree visualisation
/md-editor                   Project Markdown document editor
/keys                        API key manager
/providers                   Provider selector
/models                      Model selector
/subagents                   Subagent model selector
/codex-login                 Sign in to ChatGPT Codex
/theme                       Theme editor
/rename <title>              Rename current chat
/permission <mode>           Switch plan/edit/allow-all mode
/reasoning <level>           Set model reasoning effort
/help                        Show command help
```

## Data locations

Project-local data is written under the launch cwd:

```txt
.qubit/sessions.sqlite
.qubit/session-index.json
.qubit/runtime.log
.qubit/codex-provider-calls.log
.qubit/todos/*.md
.qubit/plans/*.md
```

User-global non-secret settings live in the platform config directory:

```txt
%APPDATA%\Qubit\settings.json        # Windows
~/.config/qubit/settings.json        # Linux
~/Library/Application Support/Qubit   # macOS
```
