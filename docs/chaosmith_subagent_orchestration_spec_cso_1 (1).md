# **Chaosmith Sub-Agent Orchestration Spec (CSO/1.0)**

## Purpose
Defines how Chaosmith spawns, seeds, manages, and recalls specialist sub-agents (routines).

## Contracts
- SpawnRequest {parentId, purpose, seed (UDCS), tools, budget, guardrails}
- SpawnReceipt {runId, routineId, status, budget, toolsBound, hashes}
- TaskMessage {runId, role, content, attachments, udcsPatch, tokensUsed}

## Lifecycle
Plan → Spawn → Execute (TaskMessages) → Verify (ATs) → Reconcile (UDCS patches) → Archive.

## Group Patterns
- Supervisor-worker
- RoundRobin
- Dynamic orchestrator
- Sleeptime

## Safety
- Risk tags [data-loss, net, priv].
- Guardrails enforce backups, deny, or revoke on breach.
- Revocation stops runner and quarantines artifacts.

## Run Report Format
```
RUN {runId}
Purpose: ...
AT: pass k/n
Artifacts: [paths]
Budget: used/limit
Risks: [tags]
Notes: ≤3 bullets
```
