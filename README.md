# Chaosmith Core

![vibe-coding](https://img.shields.io/badge/vibe_coding-heavy-orange)
![license](https://img.shields.io/badge/license-AGPLv3-blue)
![self_hosted](https://img.shields.io/badge/self--hosted-only-black)

**Chaosmith Core** is the central MCP server — a local, sovereign intelligence hub for your home-lab ecosystem.  
It indexes workspaces, embeds documents, links code and knowledge, and serves structured context to Letta-style LLM agents.  
No cloud, no telemetry — only native daemons coordinating over stdio and local transports.

---

## 🔩 Architecture

| Component | Role |
|------------|------|
| **SurrealDB** | Knowledge hoard — vector store, graphs, symbols, lists, and LSP data. |
| **NATS** | Spinal cord for async jobs and distributed executors. |
| **Core MCP** | The brainstem — exposes all MCP tools (indexer, context, graph, lists, exec). |
| **Letta / Orchestrator** | The thinking cortex, dispatching through MCP to build context and respond. |
| **Executors** | Limbs that perform heavy tasks like embedding, LSP harvest, or graph analysis. |

Everything runs as native daemons — no cloud dependencies, no Kubernetes, no third-party telemetry.

---

## ⚙️ Current tool packs

- **indexer** — scans workspaces, chunks files, embeds text/code into vectors.
- **context** — semantic context builder for RAG and multi-agent prompts.
- **lists** — lightweight workspace-bound notes, todos, ADRs.
- **graph** *(planned)* — dependency and symbol relations.
- **exec** *(planned)* — asynchronous job submission via NATS.

Each tool is exposed over MCP.  
Authentication and transport are defined by the orchestrator layer; see the Streamable HTTP endpoint section below.  
All calls produce AT (Acceptance Test) evidence for deterministic runs.

---

## 🛰️ Streamable HTTP endpoint (PCS/INST/1.0 requirement)

Chaosmith Core serves the MCP Streamable HTTP transport by default.  
Build the root module and launch it; the server listens on `:9878` (configurable via `--listen`) and exposes `/mcp`:

```bash
go build -o chaosmith-central .
./chaosmith-central --config etc/centralmcp.toml
# -> StreamableHTTP listening on :9878/mcp
```

Available tools:

- `index.workspace.scan` — walks a workspace, writes `directory`/`file` rows, and stores artifacts under `/var/lib/chaosmith/artifacts/<run_id>/`.
- `index.workspace.embed` — fetches vectors via the configured executor (`embed_url`) and upserts `vector_chunk` rows.
- `index.workspace.all` — orchestrates scan + embed in one deterministic run.
- `term_exec`, `term_pty` — retained for direct host introspection.

All three index tools return a run report (`run_id`, AT status, artifact paths, risks) so the orchestrator can enforce PCS/INST/1.0 acceptance gating.

---

## 🔌 Optional stdio bridge

If a caller insists on stdio transport (e.g., local Letta sidecar), launch with `--stdio`.  
Both transports can run simultaneously; stdio sessions share the same tool registry and SurrealDB connection.

---

## 🧠 Philosophy

> Freedom. Mastery. Chaos. Artistry.  
>  
> Chaosmith Core is built to replace cloud dependence with **local sovereignty**.  
> Everything here runs on your own iron, stores its own evidence, and answers to you alone.

---

## 🧰 Quickstart (Host‑Native MCP over TCP/WS – legacy)

Prereqs:

- Go 1.25
- Docker only for storage (SurrealDB), optional
- Ports: TCP `8777`, WS `8778`

Build:

```bash
git clone https://github.com/CryingSurrogate/chaosmith-core.git
cd chaosmith-core
go build -o bin/centralmcp ./cmd/centralmcp
```

Start storage (optional, recommended):

```bash
cd deploy
docker compose up -d
cd -
```

Run the server:

```bash
export WORK_ROOT=$PWD
export DATA_ROOT=$PWD/tmpdata
export AUTH_SECRET=change-me
export EFFECTIVE_DIM=1024
export TRANSFORM_ID=pca-nomic-v1.5-768to1024@3e24342164b3d94991ba9692fdc0dd08e3fd7362e0aacc396a9a5c54a544c3b7
export EMBED_MODEL=nomic-embed-text-v1.5
export EMBED_MODEL_SHA=sha256-of-model
export PCA_PATH=/etc/chaosmith/pca_nomic_v15_768to1024.json
export TOKENIZER_ID=tiktoken/cl100k_base
mkdir -p "$DATA_ROOT"
./bin/centralmcp --config etc/centralmcp.toml
```

Windows (PowerShell):

```powershell
# From repo root
go build -o bin\centralmcp.exe .\cmd\centralmcp

Push-Location deploy; docker compose up -d; Pop-Location

$env:WORK_ROOT = (Get-Location).Path
$env:DATA_ROOT = Join-Path $env:WORK_ROOT "tmpdata"
$env:AUTH_SECRET = "change-me"
$env:EFFECTIVE_DIM = "1024"
$env:TRANSFORM_ID = "pca-nomic-v1.5-768to1024@3e24342164b3d94991ba9692fdc0dd08e3fd7362e0aacc396a9a5c54a544c3b7"
$env:EMBED_MODEL = "nomic-embed-text-v1.5"
$env:EMBED_MODEL_SHA = "3e24342164b3d94991ba9692fdc0dd08e3fd7362e0aacc396a9a5c54a544c3b7"
$env:PCA_PATH = "C:\\etc\\chaosmith\\pca_nomic_v15_768to1024.json"
$env:TOKENIZER_ID = "tiktoken/cl100k_base"
New-Item -ItemType Directory -Force -Path $env:DATA_ROOT | Out-Null

.\bin\centralmcp.exe --config etc\centralmcp.toml
```

Endpoints:

- TCP JSON‑RPC: `0.0.0.0:8777`
- WebSocket JSON‑RPC: `ws://0.0.0.0:8778/mcp`

Authenticate first via `mcp.auth` with `{"secret":"change-me"}`.

Minimal client config (WebSocket):

```json
{ "mcpServers": { "centralmcp": { "transport": { "type": "websocket", "url": "ws://127.0.0.1:8778/mcp" } } } }
```

Smoke test (optional):

```bash
go build -o bin/e2echeck ./cmd/e2echeck
go build -o bin/e2echeckws ./cmd/e2echeckws
./bin/e2echeck      # TCP
./bin/e2echeckws    # WS
```

Systemd (Linux):

1. Install binary and config
   - `/usr/local/bin/centralmcp`
   - `/etc/chaosmith/centralmcp.toml`
   - Data dir: `/var/lib/chaosmith`
2. Unit file: `systemd/centralmcp.service`
3. `sudo systemctl enable --now centralmcp`

### Managing many workspaces

- Configure an allowlist of workspace roots with `WORK_ROOTS` (comma‑separated) or `work_roots` in `etc/centralmcp.toml`.
- Tools accept per‑call paths:
  - `index.scan { root: "/path/to/workspace" }` computes a stable `workspace_id` and scans under that root.
  - `terminal.exec { cwd: "/path/to/workspace", ... }` runs commands inside the given workspace.
  - `tty.open { cwd: "/path/to/workspace", ... }` starts an interactive shell for that workspace.
- Evidence files are stored under `${DATA_ROOT}/artifacts/<workspace_id>/<run_id>/`.

Registering workspaces (recommended):

- Register a workspace once and use `workspace_id` everywhere:

```jsonc
// JSON-RPC
{ "method": "workspace.register", "params": { "root": "/projects/foo", "name": "foo" } }
// -> { "workspace_id": "...", "root": "/projects/foo" }

// Use the id in tools
{ "method": "index.scan",  "params": { "workspace_id": "..." } }
{ "method": "terminal.exec", "params": { "workspace_id": "...", "argv": ["bash","-lc","printf x"], "capture": true } }
{ "method": "tty.open",      "params": { "workspace_id": "..." } }
```

You can still pass `cwd`/`root` directly, but `workspace_id` ensures consistent attribution and storage across runs.

### SurrealDB schema init

- Shell (Linux/macOS): `./scripts/init_surreal.sh`
- PowerShell (Windows): `./scripts/init_surreal.ps1`

Env/params:
- URL: `SURREAL_URL` or `-SurrealUrl` (default `http://127.0.0.1:8000`)
- Auth: `SURREAL_USER`/`SURREAL_PASS`
- NS/DB: `SURREAL_NS`/`SURREAL_DB`

Vector index:
- After you know the embedding dimension (see `index.index` dim), you can enable a vector index by editing `etc/schema.surql`:
  - `DEFINE INDEX doc_vector ON TABLE document FIELDS vector VECTOR DIM <your-dim> HNSW METRIC COSINE;`
  - Re-run the init script (idempotent for defines).

### PCA transform builder

Generate the PCA JSON required for CHAOS_EMB/1.0:

```bash
make build-pca
cat scripts/sample_corpus.ndjson | ./bin/build-pca \
  --endpoint http://192.168.1.64:1234/v1/embeddings \
  --model nomic-embed-text-v1.5 \
  --out /etc/chaosmith/pca_nomic_v15_768to1024.json
# note printed transform_id and set TRANSFORM_ID env/config
```


---

## 🧾 License

Chaosmith Core is licensed under the **GNU Affero General Public License v3.0**.

You are free to use, modify, and distribute this software
for any purpose, provided that all derivative works remain open-source
under the same license.

If you deploy a modified version of Chaosmith Core as a public service,
you must make the complete corresponding source available to users.

Commercial or closed-source use without an explicit license agreement
from the author is prohibited.
