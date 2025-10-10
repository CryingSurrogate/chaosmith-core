# Chaosmith Core

![vibe-coding](https://img.shields.io/badge/vibe_coding-heavy-orange)
![license](https://img.shields.io/badge/license-AGPLv3-blue)
![self\_hosted](https://img.shields.io/badge/self--hosted-only-black)

**Chaosmith Core** is the central **Model Context Protocol (MCP)** daemon — the brainstem of the Chaosmith organism.
It indexes your reality: walks files, embeds meaning, and serves structured context to Letta-style agents.
No cloud, no telemetry, no simulation — only native daemons and verified tool results.

---

## 🔩 Architecture

| Component                               | Role                                                                       |
| --------------------------------------- | -------------------------------------------------------------------------- |
| **SurrealDB**                           | Memory cortex — stores graphs, vectors, and facts.                         |
| **Core MCP (main.go)**                  | Brainstem — binds config, SurrealDB, and MCP tools.                        |
| **Embedding Executor**                  | External HTTP service (OpenAI-compatible) for vector generation.           |
| **Indexer Engine (`internal/indexer`)** | Tokenizes via `tiktoken`, writes NDJSON evidence, upserts Surreal records. |
| **Run Context (`internal/runctx`)**     | Generates deterministic `run_id`s and artifact trees.                      |
| **Executors (planned)**                 | Limbs — offload embedding, graph analysis, or workspace crawling.          |

Everything runs as **native daemons**. No containers. No Kubernetes. No third-party brokers.

---

## ⚙️ Core Tool Packs

* **indexer** — deterministic workspace scanning, chunking, embedding.
* **context** — vector and text search utilities for RAG pipelines.
* **lists** — minimal SurrealDB-backed workspace notes.
* **graph** *(planned)* — dependency and symbol relations.
* **exec** *(planned)* — remote command dispatch via NATS.

All tools expose Acceptance Test (AT) evidence as required by **PCS/INST/1.0**.
If the evidence is missing, the run is invalid — by design.

---

## 🛰️ Streamable HTTP Endpoint

Chaosmith Core serves MCP over **Streamable HTTP** by default.
Build and launch the daemon:

```bash
go build -o bin/chaosmith-mcp .
./bin/chaosmith-mcp --config etc/centralmcp.toml --listen :9878
# -> Streamable HTTP active on :9878/mcp
```

### Available Tools

* `index_workspace_scan` — walk workspace, store directory/file rows, emit artifacts under `/var/lib/chaosmith/artifacts/<run_id>/`.
* `index_workspace_embed` — chunk and embed text, upsert `vector_chunk` rows.
* `index_workspace_all` — combine scan + embed in one deterministic pass.
* `workspace_list` — list registered workspaces.
* `workspace_tree` — return directory and file tree for a workspace.
* `workspace_find_file` — find files in a workspace by exact/partial path.
* `workspace_search_text` — find exact text within workspace files.
* `file_search_text` — find exact text within a specific file.
* `file_vector_search` — vector similarity search within a file.
* `workspace_vector_search` — vector similarity search across a workspace.
* `workspace_register` — upsert a workspace bound to an existing node.
* `node_register`, `node_list` — manage/list nodes.
* `read_workspace_file` — read a file slice by character range; supports hex mode for binary-safe reads.
* `term_exec`, `term_pty` — controlled host command execution.

Each call produces a **run report** (`run_id`, AT pass/fail, artifact paths, risks) per **PCS/INST/1.0**.

---

## 🔌 Optional stdio Bridge

For Letta or local sidecar agents, you can run both transports:

```bash
./bin/chaosmith-mcp --stdio --listen :9878
```

Stdio sessions share the same SurrealDB and tool registry as HTTP.

---

## 🧠 Philosophy

> **Freedom. Mastery. Chaos. Artistry.**
>
> Chaosmith Core replaces cloud dependence with **local sovereignty**.
> Every vector, every file, every thought — stored and verified on your own iron.

---

## 🧰 Quickstart

### Prerequisites

* Go 1.25+
* SurrealDB 2.2+
* Embedding service speaking OpenAI-compatible API
* File system access to target workspaces

### Configuration

`etc/centralmcp.toml` defines connection and embedding parameters:

```toml
surreal_url  = "http://127.0.0.1:8000"
surreal_user = "root"
surreal_pass = "root"
surreal_ns   = "chaos"
surreal_db   = "core"

embed_url    = "http://127.0.0.1:1234/v1/embeddings"
embed_model  = "text-embedding-nomic-embed-text-v1.5"
effective_dim = 768
artifact_root = "var/lib/chaosmith/artifacts"
```

Override with environment variables (`SURREAL_URL`, `EMBED_URL`, etc.) or `CHAOSMITH_CONFIG`.

### Run

```bash
go run . --config etc/centralmcp.toml --listen :9878 --stdio
```

Artifacts appear under `<artifact_root>/<run_id>/` as NDJSON: `files.ndjson`, `dirs.ndjson`, `vectors.ndjson`.

---

## 🔍 Local Search Harness

Run the workspace search tool directly without MCP:

```bash
go run ./cmd/test_workspace_search --config etc/centralmcp.toml
```

---

## 🧮 MCP Tools Summary

| Category      | Tools                                                                                                                          |
| ------------- | ------------------------------------------------------------------------------------------------------------------------------ |
| **Indexing**  | `index_workspace_scan`, `index_workspace_embed`, `index_workspace_all`                                                         |
| **Inventory** | `node_register`, `node_list`, `workspace_register`, `workspace_list`, `workspace_tree`, `workspace_find_file`                 |
| **Search**    | `workspace_search_text`, `file_search_text`, `file_vector_search`, `workspace_vector_search`                                   |
| **Content**   | `read_workspace_file`                                                                                                          |
| **Terminal**  | `term_exec`, `term_pty`                                                                                                        |

All facts are derived from executors or SurrealDB — never hallucination.

---

## 🧾 License

Chaosmith Core is licensed under the **GNU Affero General Public License v3.0**.
If you deploy it as a network service, you must make your modified source available.
Commercial or closed-source use requires a separate license agreement.
