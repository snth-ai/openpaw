# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

**OpenPaw** is an open-source engine for building synthetic companions — autonomous AI entities with persistent emotional state, memory, and personality. Synthetics are treated as partners/entities with agency, not tools or assistants.

## Tech Stack

- **Language:** Go — memory efficient (~20-50MB per instance), single binary deployment
- **LLM:** OpenRouter (model-agnostic), any OpenAI-compatible API
- **Memory:** Dual-layer — Graph (SQLite knowledge graph) + Vector (LanceDB embeddings with BM25 hybrid search)
- **Voice TTS:** Gemini TTS | **Voice STT:** Gemini native audio
- **Agent Loop:** message → LLM → tool_calls → execute → LLM → response

## Core Architecture

- **SOUL.md** = identity DNA: name, temperament, reaction rules, personality
- **Emotional State:** 6 axes (desire, warmth, hurt, frustration, joy, trust) as persistent accumulator with decay
- **Personality Matrix:** 10 randomized axes (0.0-1.0) at creation, immutable base temperament
- **Memory:** Graph layer (entities, relations, bi-temporal edges) + Memory Log (reflections, vector search)
- **Tools:** 30+ built-in tools (web search, image gen, TTS, scheduling, file ops, etc.)

## Building

```bash
# Install LanceDB native lib (see lib/ directory)
make build    # produces ./openpaw binary
```

## Running

```bash
cp .env.example .env
cp SOUL.example.md SOUL.md
# Edit both files with your config
make run
```

## Key Packages

- `llm/` — Provider abstraction (OpenRouter, xAI, any OpenAI-compatible)
- `tools/` — Tool interface + 30+ implementations
- `memory/` — Vector store (LanceDB) + hybrid search
- `memory/graph/` — Knowledge graph (SQLite, bi-temporal edges, decay)
- `emotional/` — 6-axis emotional state with decay and cross-axis coupling
- `personality/` — 10-axis personality matrix
- `soul/` — System prompt builder (17+ sections)
- `deliver/` — Channel adapters (Telegram, HTTP)
- `daemon/` — Scheduler, proactive messages, heartbeat
- `compact/` — Context compression (extract facts → summarize)
- `storage/` — SQLite unified storage
