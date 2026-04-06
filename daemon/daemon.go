package daemon

import (
	"log"
	"sync"
	"time"

	"github.com/openpaw/server/uid"
)

// TaskFunc — функция задачи. Возвращает текст для проактивного сообщения (или "" если молчим).
type TaskFunc func() string

// Task — запланированная задача.
type Task struct {
	ID       string
	Name     string
	Func     TaskFunc
	Interval time.Duration // 0 = one-shot
	NextRun  time.Time
	Repeat   bool
}

// ProactiveMessage — сообщение которое синт хочет отправить сам.
type ProactiveMessage struct {
	SessionID string
	Text      string
	CreatedAt time.Time
}

// PersistCallbacks — hooks для persistence задач.
type PersistCallbacks struct {
	OnSave   func(id, name, message, sessionID string, interval time.Duration, repeat bool, nextRun time.Time) // вызывается при Schedule
	OnDelete func(id string) // вызывается при Cancel / one-shot завершении
}

// Daemon — планировщик фоновых задач и проактивных сообщений.
type Daemon struct {
	mu          sync.Mutex
	tasks       map[string]*Task
	outbox      []ProactiveMessage
	stop        chan struct{}
	tick        time.Duration
	onProactive func(ProactiveMessage)
	persist     *PersistCallbacks
}

func New(tickInterval time.Duration, persist *PersistCallbacks) *Daemon {
	if tickInterval == 0 {
		tickInterval = 10 * time.Second
	}
	return &Daemon{
		tasks:   make(map[string]*Task),
		stop:    make(chan struct{}),
		tick:    tickInterval,
		persist: persist,
	}
}

// OnProactive устанавливает callback для проактивных сообщений.
func (d *Daemon) OnProactive(fn func(ProactiveMessage)) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.onProactive = fn
}

// Schedule добавляет задачу. Если repeat=true, повторяется каждые interval.
func (d *Daemon) Schedule(name string, interval time.Duration, repeat bool, fn TaskFunc) string {
	d.mu.Lock()
	defer d.mu.Unlock()

	id := uid.New()
	d.tasks[id] = &Task{
		ID:       id,
		Name:     name,
		Func:     fn,
		Interval: interval,
		NextRun:  time.Now().Add(interval),
		Repeat:   repeat,
	}

	log.Printf("daemon: scheduled %q (id: %s, interval: %s, repeat: %v)", name, id, interval, repeat)
	return id
}

// ScheduleWithMeta — Schedule с метаданными для persistence.
func (d *Daemon) ScheduleWithMeta(name, message, sessionID string, interval time.Duration, repeat bool, fn TaskFunc) string {
	id := d.Schedule(name, interval, repeat, fn)

	if d.persist != nil && d.persist.OnSave != nil {
		d.persist.OnSave(id, name, message, sessionID, interval, repeat, time.Now().Add(interval))
	}

	return id
}

// RestoreTask восстанавливает задачу после рестарта (без persist callback).
func (d *Daemon) RestoreTask(id, name string, interval time.Duration, nextRun time.Time, repeat bool, fn TaskFunc) {
	d.mu.Lock()
	defer d.mu.Unlock()

	// Если nextRun в прошлом — запустим сразу
	if nextRun.Before(time.Now()) {
		nextRun = time.Now().Add(1 * time.Second)
	}

	d.tasks[id] = &Task{
		ID:       id,
		Name:     name,
		Func:     fn,
		Interval: interval,
		NextRun:  nextRun,
		Repeat:   repeat,
	}

	log.Printf("daemon: restored %q (id: %s, next: %s)", name, id, nextRun.Format("15:04:05"))
}

// Cancel отменяет задачу по ID.
func (d *Daemon) Cancel(id string) bool {
	d.mu.Lock()
	defer d.mu.Unlock()

	if _, ok := d.tasks[id]; ok {
		delete(d.tasks, id)
		log.Printf("daemon: cancelled task %s", id)

		if d.persist != nil && d.persist.OnDelete != nil {
			d.persist.OnDelete(id)
		}
		return true
	}
	return false
}

// List возвращает все активные задачи.
func (d *Daemon) List() []Task {
	d.mu.Lock()
	defer d.mu.Unlock()

	out := make([]Task, 0, len(d.tasks))
	for _, t := range d.tasks {
		out = append(out, *t)
	}
	return out
}

// SendProactive кладёт проактивное сообщение в outbox.
func (d *Daemon) SendProactive(sessionID, text string) {
	d.mu.Lock()
	defer d.mu.Unlock()

	msg := ProactiveMessage{
		SessionID: sessionID,
		Text:      text,
		CreatedAt: time.Now(),
	}

	if d.onProactive != nil {
		d.onProactive(msg)
	} else {
		d.outbox = append(d.outbox, msg)
	}

	log.Printf("daemon: proactive message for session %s: %s", sessionID, truncate(text, 80))
}

// DrainOutbox забирает все накопленные проактивные сообщения.
func (d *Daemon) DrainOutbox() []ProactiveMessage {
	d.mu.Lock()
	defer d.mu.Unlock()

	out := d.outbox
	d.outbox = nil
	return out
}

// DrainForSession забирает проактивные сообщения для конкретной сессии.
func (d *Daemon) DrainForSession(sessionID string) []ProactiveMessage {
	d.mu.Lock()
	defer d.mu.Unlock()

	var keep, drain []ProactiveMessage
	for _, m := range d.outbox {
		if m.SessionID == sessionID {
			drain = append(drain, m)
		} else {
			keep = append(keep, m)
		}
	}
	d.outbox = keep
	return drain
}

// Start запускает тик-цикл daemon в горутине.
func (d *Daemon) Start() {
	go d.loop()
	log.Printf("daemon: started (tick: %s)", d.tick)
}

// Stop останавливает daemon.
func (d *Daemon) Stop() {
	close(d.stop)
	log.Println("daemon: stopped")
}

func (d *Daemon) loop() {
	ticker := time.NewTicker(d.tick)
	defer ticker.Stop()

	for {
		select {
		case <-d.stop:
			return
		case now := <-ticker.C:
			d.runDue(now)
		}
	}
}

func (d *Daemon) runDue(now time.Time) {
	d.mu.Lock()
	var due []*Task
	for _, t := range d.tasks {
		if now.After(t.NextRun) || now.Equal(t.NextRun) {
			due = append(due, t)
		}
	}
	d.mu.Unlock()

	for _, t := range due {
		result := t.Func()
		if result != "" {
			log.Printf("daemon: task %q produced: %s", t.Name, truncate(result, 80))
		}

		d.mu.Lock()
		if t.Repeat {
			t.NextRun = now.Add(t.Interval)
		} else {
			delete(d.tasks, t.ID)
			// One-shot завершился — удаляем из persistence
			if d.persist != nil && d.persist.OnDelete != nil {
				d.persist.OnDelete(t.ID)
			}
		}
		d.mu.Unlock()
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
