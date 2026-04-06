# Compact — Context Compression

## Overview

When conversation history exceeds ~80K characters (~20K tokens), old messages are compressed to prevent context window overflow. Before compression, key facts are extracted and saved to long-term memory so nothing important is lost.

## Flow

```
chatHandler receives messages
  │
  ├─ compact.NeedsCompact(messages) → total chars > 80K?
  │    └─ No → skip, proceed normally
  │    └─ Yes ↓
  │
  ├─ Compactor.Compact(messages):
  │    │
  │    ├─ Split: [system] + [old messages] + [last 20 messages]
  │    │
  │    ├─ Step 0 (NEW): Graph extraction from old messages (async)
  │    │    ├─ graphPipeline.ProcessDialogue(oldMessages)
  │    │    ├─ LLM extracts entities + triplets + invalidations
  │    │    ├─ Entity resolution (3-phase) → graph_nodes
  │    │    ├─ Create/update edges → graph_edges
  │    │    └─ Runs in goroutine, doesn't block compact
  │    │
  │    ├─ Step 1: Extract facts from old messages
  │    │    ├─ LLM prompt: "extract key facts as JSON array"
  │    │    ├─ Each fact: {text, category, importance}
  │    │    ├─ Filter: importance >= 0.3
  │    │    ├─ Generate embedding for each fact
  │    │    └─ Save to Memory Log (LanceDB)
  │    │
  │    ├─ Step 2: Summarize old messages
  │    │    ├─ LLM prompt: "summarize preserving topics, tone, unresolved threads"
  │    │    └─ Returns concise summary in conversation language
  │    │
  │    └─ Result: [system] + [<conversation-summary>] + [last 20 messages]
  │
  ├─ Save compacted history to SQLite session
  └─ Proceed with agent loop using shorter context
```

## Why Extract Before Summarize

Summaries are lossy — they compress further on next compact, eventually losing details. By extracting before summarizing:
- **Graph**: entities and relations survive as structured knowledge (graph_nodes + graph_edges)
- **Memory Log**: facts survive with importance decay, recalled via embedding similarity
- **Summary**: only needs to preserve conversational flow, not archival data
- Graph extraction is the PRIMARY way the knowledge graph grows (batch of 20-50 messages = high quality)

## Package: `compact/`

### compact.go
- `NeedsCompact(messages) bool` — checks total char count > MaxChars (80K)
- `Compactor` struct: needs Provider (LLM), Store (memory), Embedder
- `Compact(messages) → []llm.Message` — full flow: extract + summarize + rebuild
- `extractFacts(old)` — LLM extracts JSON array of facts → embed → memory_store
- `summarize(old)` — LLM produces concise summary

### Constants
- `MaxChars = 80000` (~20K tokens threshold)
- `KeepRecent = 20` (last 20 messages kept verbatim)

## Error Handling

- If fact extraction fails → log warning, continue with summarization
- If summarization fails → return original messages unchanged (no data loss)
- Non-fatal: compact is optimization, not requirement

## What's NOT Implemented Yet

- Recursive compact (summary of summaries)
- Token-accurate counting (currently char-based estimate)
- Configurable thresholds via env/config
- Async compact (currently blocks request)
