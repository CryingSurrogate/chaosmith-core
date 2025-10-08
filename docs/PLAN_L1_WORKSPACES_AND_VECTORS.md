# Chaosmith L1: Workspaces + Vectors — Implementation Plan (PCS/1.3-native, UDCS/1.0, CSO/1.0)

This plan turns the current repo into a functioning L1 indexer + embedder wired to our MCP brain. No cloud. No containers. Only daemons.

## Goals (L1 scope)
- Represent dens, nodes, workspaces, directories, files, symbols per `schema.surql`.
- Index files/dirs, extract symbols, create containment/defines edges.
- Generate vectors for symbol/file spans; store provenance + vectors.
- All facts originate in executors; MCP only orchestrates.
- Emit UDCS/1.0 run summaries with AT evidence.

## Repo impact
```
chaosmith-core/
  cmd/
    chaos-indexer/            # executor: scan/symbols/embed
    chaos-orch/               # orchestrator CLI helpers (optional)
  internal/
    surreal/                  # minimal Surreal client (/sql)
    fswalk/                   # file walk, hashing (BLAKE3/SHA-256)
    langmap/                  # ext→lang map + overrides
    symbols/                  # ctags JSON normalize
    embed/                    # local model client (LM Studio/Ollama)
    udcs/                     # UDCS patch builder
    at/                       # acceptance tests
  etc/
    schema.surql              # already present
    centralmcp.toml           # MCP config (add tools here)
  tools/
    term_exec.go              # existing; keep
```

## Milestones
1. **M0 — DB Bootstrap**
   - Load `etc/schema.surql` via `scripts/init_surreal.{sh,ps1}`.
   - Smoke-test: upsert a den/node/workspace; create `on_node` and `den_has_*` edges.

2. **M1 — L0 Index (dirs/files)**
   - `cmd/chaos-indexer scan`: walk workspace, produce `files.jsonl`, upsert: `directory`, `file`, edges `ws_contains_dir`, `dir_contains_dir`, `dir_contains_file`.
   - Hashing: default BLAKE3; allow `--sha256`.
   - ATs: discovered==inserted; root `ws_contains_dir` nonzero.

3. **M2 — L1 Symbols**
   - `cmd/chaos-indexer symbols`: run `universal-ctags` JSON; normalize `{name,fqname,kind,lang,range}`; upsert `symbol`; edges `file_contains_sym`, `defines`.
   - ATs: sample ranges point to correct identifier; files with symbols have `defines`.

4. **M3 — Vectors**
   - `cmd/chaos-indexer embed`: chunk by `symbol` (code) or `file` (text); compute `content_sha`; call local embedding server; upsert `vector_model`, `vector_chunk`; relate `symbol_has_vector` or `file_has_vector`.
   - Compute centroid → `workspace_vector` + `workspace_has_vector`.
   - ATs: `len(vector)` matches dim; `content_sha` recomputes; centroid sample > 0.

5. **M4 — UDCS Patches**
   - Emit UDCS/1.0 run summary with stats and artifact paths. Append to Surreal memory block `project_index`.

6. **M5 — MCP Tools**
   - Expose orchestrated flows via MCP:
     - `index.workspace.scan`
     - `index.workspace.symbols`
     - `index.workspace.embed`
     - `index.workspace.all`
     - `inventory.den`
     - `udcs.emit.run_summary`
