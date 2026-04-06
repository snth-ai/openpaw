package tools

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/openpaw/server/daemon"
)

// Schedule — тул для синта: запланировать задачу или напоминание.
type Schedule struct {
	daemon    *daemon.Daemon
	sessionID string
}

func NewSchedule(d *daemon.Daemon) *Schedule {
	return &Schedule{daemon: d}
}

func (t *Schedule) SetContext(ctx CallContext) {
	t.sessionID = ctx.SessionID
}

func (t *Schedule) Name() string { return "schedule" }
func (t *Schedule) Description() string {
	return "Schedule a task or reminder. Can be one-shot (e.g., remind in 2 hours) or repeating (e.g., check every day). When the task fires, a proactive message is sent to the current user."
}

func (t *Schedule) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"name": {
				"type": "string",
				"description": "Short name for the task (e.g., 'remind about meeting', 'daily check-in')"
			},
			"delay": {
				"type": "string",
				"description": "When to run: duration format. Examples: '30m', '2h', '24h', '168h' (1 week)"
			},
			"repeat": {
				"type": "boolean",
				"description": "If true, repeats every 'delay' interval. If false, runs once."
			},
			"message": {
				"type": "string",
				"description": "Message to send to the user when the task fires"
			}
		},
		"required": ["name", "delay", "message"]
	}`)
}

func (t *Schedule) Execute(args json.RawMessage) (string, error) {
	var params struct {
		Name    string `json:"name"`
		Delay   string `json:"delay"`
		Repeat  bool   `json:"repeat"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return "", fmt.Errorf("parse args: %w", err)
	}

	dur, err := time.ParseDuration(params.Delay)
	if err != nil {
		return "", fmt.Errorf("invalid delay %q: %w", params.Delay, err)
	}

	msg := params.Message
	sid := t.sessionID

	id := t.daemon.ScheduleWithMeta(params.Name, msg, sid, dur, params.Repeat, func() string {
		t.daemon.SendProactive(sid, msg)
		return msg
	})

	repeatStr := "one-shot"
	if params.Repeat {
		repeatStr = fmt.Sprintf("every %s", dur)
	}

	return fmt.Sprintf("Scheduled %q (id: %s, %s, fires at %s)",
		params.Name, id, repeatStr, time.Now().Add(dur).Format("15:04:05")), nil
}

// ScheduleCancel — тул для отмены задачи.
type ScheduleCancel struct {
	daemon *daemon.Daemon
}

func NewScheduleCancel(d *daemon.Daemon) *ScheduleCancel {
	return &ScheduleCancel{daemon: d}
}

func (t *ScheduleCancel) Name() string { return "schedule_cancel" }
func (t *ScheduleCancel) Description() string {
	return "Cancel a scheduled task by its ID."
}

func (t *ScheduleCancel) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"id": {
				"type": "string",
				"description": "Task ID to cancel"
			}
		},
		"required": ["id"]
	}`)
}

func (t *ScheduleCancel) Execute(args json.RawMessage) (string, error) {
	var params struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return "", fmt.Errorf("parse args: %w", err)
	}

	if t.daemon.Cancel(params.ID) {
		return fmt.Sprintf("Task %s cancelled.", params.ID), nil
	}
	return fmt.Sprintf("Task %s not found.", params.ID), nil
}

// ScheduleList — тул для просмотра активных задач.
type ScheduleList struct {
	daemon *daemon.Daemon
}

func NewScheduleList(d *daemon.Daemon) *ScheduleList {
	return &ScheduleList{daemon: d}
}

func (t *ScheduleList) Name() string { return "schedule_list" }
func (t *ScheduleList) Description() string {
	return "List all active scheduled tasks."
}

func (t *ScheduleList) Parameters() json.RawMessage {
	return json.RawMessage(`{"type": "object", "properties": {}}`)
}

func (t *ScheduleList) Execute(_ json.RawMessage) (string, error) {
	tasks := t.daemon.List()
	if len(tasks) == 0 {
		return "No active tasks.", nil
	}

	var sb strings.Builder
	for _, task := range tasks {
		repeatStr := "once"
		if task.Repeat {
			repeatStr = fmt.Sprintf("every %s", task.Interval)
		}
		sb.WriteString(fmt.Sprintf("- %s (id: %s, %s, next: %s)\n",
			task.Name, task.ID, repeatStr, task.NextRun.Format("15:04:05")))
	}
	return sb.String(), nil
}
