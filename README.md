# Chaosmith Core

**Chaosmith Core** is the central MCP server — a local, sovereign intelligence hub for your home-lab ecosystem.  
It indexes workspaces, embeds documents, links code and knowledge, and serves structured context to Letta-style LLM agents.  
No cloud, no telemetry, no stdio dependency. Just pure daemons talking over TCP.

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

Each tool is exposed over JSON-RPC 2.0 (TCP or WebSocket).  
Authentication is a shared-secret HMAC session handshake.  
All calls produce AT (Acceptance Test) evidence for deterministic runs.

---

## 🧠 Philosophy

> Freedom. Mastery. Chaos. Artistry.  
>  
> Chaosmith Core is built to replace cloud dependence with **local sovereignty**.  
> Everything here runs on your own iron, stores its own evidence, and answers to you alone.

---

## 🧰 Quick start

```bash
git clone https://github.com/<you>/chaosmith-core.git
cd chaosmith-core
docker compose up -d --build
