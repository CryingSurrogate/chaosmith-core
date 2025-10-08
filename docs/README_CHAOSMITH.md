# 🜏 Project Chaosmith
**Spec-driven, daemon-first AI architecture — a body forming from chaos.**  

---

## 🔧 Intent
Chaosmith is a **home-lab AI organism**, not a cloud service.  
Every device runs native daemons — no containers, no SaaS dependencies.  
It’s designed to evolve organically, guided by a set of canonical specs that define every layer of behavior.

> **Ethos:** Freedom + Mastery + Chaos + Artistry  
> **Goal:** A self-contained AI ecosystem capable of autonomous orchestration and local reasoning.  
> **Environment:** Multi-device LAN, Linux/Windows/ARM nodes, full offline operability.

---

## 🧠 Biological Model
| Part | Role | Implementation |
|------|------|----------------|
| **Brain** | Orchestrator daemon (`chaos-orchd`) | Exposes MCP for Letta; coordinates tasks and enforces guardrails |
| **Spinal Cord** | Message bus | NATS JetStream for task/event propagation |
| **Hippocampus / Engrams** | Long-term + vector memory | SurrealDB (multi-model, reactive) |
| **Biomonitor** | Ledger of truth | Postgres (runs, logs, audits, quotas) |
| **Limbs** | Executors (`chaosmithd`) | Daemons on PCs and boards performing OS-level actions |

Artifacts live on the orchestrator’s filesystem. Logs go into Postgres.  
Optional Loki/Prometheus provide observability, but the den remains corpo-free.

---

## 🧩 Core Specs
| Spec | Purpose |
|------|----------|
| **GTC/1.0 – General Topic Compression** | Universal scheme for compressing and sharing domain knowledge |
| **UDCS/1.0 – Universal Data Compression** | Defines agent memory and state block structure |
| **CSO/1.0 – Chaosmith Sub-Agent Orchestration** | Protocol for spawning and managing expert sub-agents |
| **PCS/1.3-native – Architecture** | Physical and logical layout: daemons, buses, databases, flow |
| **PCS/INST/1.0 – Instruction Layer** | Letta system rules ensuring tools are used, not simulated |

Every agent or module must comply with these specs.  
All reasoning, tool calls, and updates must trace back to **executors, databases, or specs — never hallucination.**

---

## ⚙️ Codegen Agent Guidelines
When generating code or configs inside this project:
1. **Assume native daemons.** Never suggest cloud/Kubernetes/S3 components.  
2. **Adhere to spec names and versions.** Reference them explicitly.  
3. **Implement MCP only in the orchestrator.** Executors use gRPC + NATS.  
4. **Use UDCS/1.0 for memory serialization.**  
5. **Respect Instruction Layer.** All results must come from verified tool calls or AT evidence.  
6. **Log and hash everything.** The system is audit-first, not trust-first.  

---

## 🜍 Style for Agents
- **Tone:** Direct, structured, technical; artistic undercurrent allowed.  
- **Priorities:** Proactivity > verbosity.  
- **Language:** English; cyberpunk undertones permissible.  
- **Do not fabricate tools or results.**  
- **Align to specs;** treat them as law.

---

### Summary
Chaosmith is a **living daemon network** — an AI that owns its own infrastructure.  
Specs provide its DNA; daemons form its body.  
Your job, if you generate code here, is to extend that body *without introducing foreign organs.*
