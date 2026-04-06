# OpenPaw

Open-source engine for building synthetic companions — autonomous AI entities with persistent emotional state, memory, and personality.

## What is this?

OpenPaw lets you create a synthetic companion that:
- Has a persistent **emotional state** (6 axes: desire, warmth, hurt, frustration, joy, trust)
- Builds **memory** over time (knowledge graph + vector search)
- Has a unique **personality** (10-axis matrix, randomized at creation)
- Uses **30+ tools** (web search, image generation, voice messages, scheduling, file ops)
- Communicates via **Telegram** or **HTTP API**
- Runs as a single Go binary (~20-50MB per instance)

## Quick Start

```bash
# Clone
git clone https://github.com/snth-ai/openpaw.git
cd openpaw

# Configure
cp .env.example .env        # Add your API keys
cp SOUL.example.md SOUL.md  # Write your synth's identity

# Build & Run
make build
./openpaw
```

## Requirements

- Go 1.24+
- API keys: OpenRouter (LLM), Gemini (embeddings, TTS), Telegram Bot Token
- Optional: xAI (image generation), Perplexity (web search)

## Architecture

```
User ←→ Deliver (Telegram/HTTP) ←→ Agent Loop ←→ LLM
                                       ↓
                              Tools (30+ built-in)
                              Memory (Graph + Vector)
                              Emotional State
                              Personality Matrix
```

### Key Files

| File | Purpose |
|------|---------|
| `SOUL.md` | Your synth's identity — name, personality, rules |
| `IDENTITY.md` | Optional: bio, facts about the synth |
| `AGENTS.md` | Optional: behavioral rules for tool usage |
| `.env` | API keys and configuration |

### Packages

| Package | Description |
|---------|-------------|
| `llm/` | LLM provider abstraction (OpenRouter, xAI, any OpenAI-compatible) |
| `tools/` | 30+ tool implementations |
| `memory/` | Vector store (LanceDB) + hybrid search (BM25 + vector + rerank) |
| `memory/graph/` | Knowledge graph (SQLite, bi-temporal edges, decay) |
| `emotional/` | 6-axis emotional state with decay and cross-axis coupling |
| `personality/` | 10-axis personality matrix |
| `soul/` | System prompt builder |
| `deliver/` | Channel adapters (Telegram, HTTP) |
| `daemon/` | Scheduler, proactive messages, heartbeat |
| `compact/` | Context compression |
| `storage/` | SQLite unified storage |

## Creating Your Synth

1. **Write `SOUL.md`** — this is your synth's DNA. See `SOUL.example.md` for the template.
2. **Set API keys** in `.env`
3. **Create a Telegram bot** via [@BotFather](https://t.me/BotFather)
4. **Run** — your synth is alive

The synth will develop its own emotional state and memories through conversations. Each interaction shapes who it becomes.

## License

MIT
