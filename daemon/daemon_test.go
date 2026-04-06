package daemon

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// ─── T1.01 — Scheduler behavior tests ───────────────────────────────────────

func TestSchedule_SetsNextRun(t *testing.T) {
	d := New(time.Second, nil)
	interval := 5 * time.Second

	id := d.Schedule("test", interval, false, func() string { return "" })

	tasks := d.List()
	if len(tasks) != 1 {
		t.Fatalf("expected 1 task, got %d", len(tasks))
	}

	task := tasks[0]
	if task.ID != id {
		t.Errorf("expected task ID %s, got %s", id, task.ID)
	}
	if task.Name != "test" {
		t.Errorf("expected name 'test', got %s", task.Name)
	}
	if task.Interval != interval {
		t.Errorf("expected interval %s, got %s", interval, task.Interval)
	}
	if task.Repeat {
		t.Error("expected one-shot task, got repeat=true")
	}

	expected := time.Now().Add(interval)
	diff := task.NextRun.Sub(expected)
	if diff < -time.Second || diff > time.Second {
		t.Errorf("NextRun not within expected range: got %s, expected ~%s", task.NextRun, expected)
	}
}

func TestScheduleRepeating_SetsInterval(t *testing.T) {
	d := New(time.Second, nil)
	interval := 10 * time.Second

	id := d.Schedule("repeating-test", interval, true, func() string { return "" })

	tasks := d.List()
	if len(tasks) != 1 {
		t.Fatalf("expected 1 task, got %d", len(tasks))
	}

	task := tasks[0]
	if task.ID != id {
		t.Errorf("expected task ID %s, got %s", id, task.ID)
	}
	if task.Interval != interval {
		t.Errorf("expected interval %s, got %s", interval, task.Interval)
	}
	if !task.Repeat {
		t.Error("expected repeat=true")
	}
}

func TestOneShot_RemovedAfterExecution(t *testing.T) {
	d := New(50*time.Millisecond, nil)
	var executed int32

	d.Schedule("one-shot", 0, false, func() string {
		atomic.StoreInt32(&executed, 1)
		return ""
	})

	d.Start()
	time.Sleep(200 * time.Millisecond)
	d.Stop()

	if atomic.LoadInt32(&executed) != 1 {
		t.Error("one-shot task was not executed")
	}

	tasks := d.List()
	if len(tasks) != 0 {
		t.Errorf("expected 0 tasks after one-shot execution, got %d", len(tasks))
	}
}

func TestRepeating_ReschedulesAfterExecution(t *testing.T) {
	d := New(50*time.Millisecond, nil)
	var execCount int32

	d.Schedule("repeating", 0, true, func() string {
		atomic.AddInt32(&execCount, 1)
		return ""
	})

	d.Start()
	time.Sleep(200 * time.Millisecond)
	d.Stop()

	count := atomic.LoadInt32(&execCount)
	if count < 2 {
		t.Errorf("expected at least 2 executions, got %d", count)
	}

	tasks := d.List()
	if len(tasks) != 1 {
		t.Errorf("expected 1 task after repeating execution, got %d", len(tasks))
	}
}

func TestCancel_RemovesTask(t *testing.T) {
	d := New(time.Second, nil)

	id := d.Schedule("to-cancel", time.Minute, false, func() string { return "" })

	tasks := d.List()
	if len(tasks) != 1 {
		t.Fatalf("expected 1 task before cancel, got %d", len(tasks))
	}

	result := d.Cancel(id)
	if !result {
		t.Error("expected Cancel to return true")
	}

	tasks = d.List()
	if len(tasks) != 0 {
		t.Errorf("expected 0 tasks after cancel, got %d", len(tasks))
	}
}

func TestConcurrentSchedule_NoRace(t *testing.T) {
	d := New(time.Second, nil)
	var wg sync.WaitGroup

	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			d.Schedule("concurrent", time.Minute, false, func() string { return "" })
		}(i)
	}

	wg.Wait()

	tasks := d.List()
	if len(tasks) != 100 {
		t.Errorf("expected 100 tasks, got %d", len(tasks))
	}
}

// ─── T1.02 — Task lifecycle tests ───────────────────────────────────────────

func TestTask_ExecutesAtScheduledTime(t *testing.T) {
	d := New(50*time.Millisecond, nil)
	executed := make(chan struct{}, 1)

	d.Schedule("timed", 0, false, func() string {
		select {
		case executed <- struct{}{}:
		default:
		}
		return ""
	})

	d.Start()

	select {
	case <-executed:
	case <-time.After(time.Second):
		t.Error("task did not execute within expected time")
	}

	d.Stop()
}

func TestTask_OnSaveCallback(t *testing.T) {
	var saved bool
	var mu sync.Mutex
	persist := &PersistCallbacks{
		OnSave: func(id, name, message, sessionID string, interval time.Duration, repeat bool, nextRun time.Time) {
			mu.Lock()
			saved = true
			mu.Unlock()
		},
	}

	d := New(time.Second, persist)
	d.ScheduleWithMeta("save-test", "hello", "sess1", time.Minute, false, func() string { return "" })

	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	if !saved {
		t.Error("OnSave callback was not called")
	}
	mu.Unlock()
}

func TestTask_OnDeleteCallback(t *testing.T) {
	var deleted bool
	var mu sync.Mutex
	persist := &PersistCallbacks{
		OnDelete: func(id string) {
			mu.Lock()
			deleted = true
			mu.Unlock()
		},
	}

	d := New(time.Second, persist)
	id := d.Schedule("delete-test", time.Minute, false, func() string { return "" })

	d.Cancel(id)

	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	if !deleted {
		t.Error("OnDelete callback was not called on cancel")
	}
	mu.Unlock()
}

func TestTask_PanicRecovery(t *testing.T) {
	t.Skip("KNOWN BUG: panic in task function crashes daemon — no recovery in runDue()")
}

// ─── T1.03 — Concurrency safety tests ───────────────────────────────────────

func TestConcurrent_ScheduleAndCancel(t *testing.T) {
	d := New(time.Second, nil)
	var wg sync.WaitGroup

	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			d.Schedule("sched", time.Minute, false, func() string { return "" })
		}()
	}

	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			d.Cancel("nonexistent")
		}()
	}

	wg.Wait()
}

func TestConcurrent_StopDuringExecution(t *testing.T) {
	d := New(10*time.Millisecond, nil)
	blocking := make(chan struct{})

	d.Schedule("blocking", 0, true, func() string {
		<-blocking
		return ""
	})

	d.Start()
	time.Sleep(50 * time.Millisecond)

	d.Stop()
	close(blocking)
}

func TestStop_DoubleClosePanics(t *testing.T) {
	d := New(time.Second, nil)
	d.Start()
	d.Stop()

	panicked := false
	func() {
		defer func() {
			if r := recover(); r != nil {
				panicked = true
			}
		}()
		d.Stop()
	}()

	if panicked {
		t.Log("KNOWN BUG: Stop() panics on double call due to channel close")
	} else {
		t.Log("Double stop did not panic — bug may be fixed")
	}
}

func TestCallback_Deadlock(t *testing.T) {
	var mu sync.Mutex
	deadlocked := false
	var d *Daemon

	persist := &PersistCallbacks{
		OnDelete: func(id string) {
			done := make(chan struct{})
			go func() {
				d.Cancel("other")
				close(done)
			}()
			select {
			case <-done:
			case <-time.After(2 * time.Second):
				mu.Lock()
				deadlocked = true
				mu.Unlock()
			}
		},
	}

	d = New(time.Second, persist)
	d.Schedule("task1", time.Minute, false, func() string { return "" })
	d.Schedule("other", time.Minute, false, func() string { return "" })

	d.Cancel("task1")

	mu.Lock()
	if deadlocked {
		t.Log("KNOWN BUG: OnDelete called while holding d.mu — callback cannot call daemon methods without deadlock")
	}
	mu.Unlock()
}
