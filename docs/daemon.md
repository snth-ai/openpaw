# Daemon — Scheduler & Proactive Messages

## Overview

Background task scheduler. Runs in a goroutine, ticks every 10 seconds, executes due tasks. Supports one-shot and repeating tasks. Enables proactive messages — synth can initiate contact without user request.

## Architecture

```
Daemon goroutine (tick every 10s)
  │
  ├─ Check all tasks: is now >= task.NextRun?
  │    ├─ Yes → execute task.Func()
  │    │    ├─ If repeat → reschedule (NextRun += Interval)
  │    │    └─ If one-shot → remove from task map
  │    └─ No → skip
  │
  └─ Built-in tasks:
       ├─ memory-decay (every 1h) — runs memStore.RunDecay()
       └─ daily-digest (every 24h) — graph extraction from all sessions + diary entry to Memory Log
```

## Proactive Messages Flow

```
1. Synth calls schedule tool: "remind in 2h" → daemon.Schedule(...)
2. 2 hours later, daemon fires task → daemon.SendProactive(sessionID, message)
3. Message goes to outbox (in-memory queue)
4. Client polls GET /proactive?session_id=xxx → drains messages
   OR next POST /chat includes pending proactive messages in response
```

## Package: `daemon/`

### daemon.go
- `Daemon` struct: task map, outbox, tick interval, stop channel
- `New(tickInterval)` — create daemon (default 10s tick)
- `Start()` — launches goroutine
- `Stop()` — signals goroutine to exit
- `Schedule(name, interval, repeat, func) → taskID` — add task
- `Cancel(id) → bool` — remove task
- `List() → []Task` — active tasks
- `SendProactive(sessionID, text)` — queue proactive message
- `DrainOutbox() → []ProactiveMessage` — get all pending
- `DrainForSession(sessionID) → []ProactiveMessage` — get pending for specific session
- `OnProactive(callback)` — optional callback instead of outbox (for future WebSocket/push)

### Task struct
- ID, Name, Func (returns string), Interval, NextRun, Repeat

### ProactiveMessage struct
- SessionID, Text, CreatedAt

## Tools (in `tools/schedule.go`)

### schedule (ContextAware)
- Synth schedules a task with name, delay (Go duration), repeat flag, message
- session_id injected automatically via CallContext (synth doesn't need to know it)
- Returns task ID and fire time

### schedule_cancel
- Cancel task by ID

### schedule_list
- List all active tasks with name, ID, repeat info, next run time

## ContextAware Interface (`tools/tool.go`)

Tools that need session context implement `ContextAware`:
```go
type ContextAware interface {
    SetContext(ctx CallContext)
}
type CallContext struct {
    SessionID string
}
```
`Registry.SetContext(ctx)` is called before each agent loop in chatHandler, propagating session_id to all ContextAware tools.

## HTTP Endpoints

### GET /proactive?session_id=xxx
- Returns JSON array of pending proactive messages for that session
- Drains messages (each message returned only once)
- Response: `[{"text":"...", "created_at":"2026-03-25T11:34:38+08:00"}]`

### POST /chat (updated)
- Response now includes `proactive` field with any pending messages that accumulated since last request
- `{"reply":"...", "session_id":"...", "proactive":[{"text":"...", "created_at":"..."}]}`

## Built-in Scheduled Tasks

- **memory-decay** — runs every 1 hour, applies importance decay to all memories, deletes below threshold
- **daily-digest** — runs every 24 hours. Performs graph extraction from all sessions accumulated that day (catches knowledge from sessions that never hit compact threshold). Also writes a diary entry to Memory Log summarizing the day.

## What's NOT Implemented Yet

- Persistent task storage (tasks lost on restart)
- WebSocket/SSE push for proactive messages (currently polling)
- Heartbeat task (periodic "thinking of you" / check-in)
- Complex cron expressions (currently only fixed intervals)
