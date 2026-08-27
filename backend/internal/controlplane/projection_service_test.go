package controlplane

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/VBenevides/Glyphflow/backend/internal/platform"
	"github.com/VBenevides/Glyphflow/backend/internal/store"
)

type projectionRepositoryFunc func(context.Context) ([]store.ScheduleProjectionInput, error)

func (f projectionRepositoryFunc) ListScheduleProjection(ctx context.Context) ([]store.ScheduleProjectionInput, error) {
	return f(ctx)
}

func projectionTestInput() []store.ScheduleProjectionInput {
	return []store.ScheduleProjectionInput{{
		ScheduleID: "schedule-1", TaskID: "task-1", TaskVersionID: "task-1-v1", Expression: "0 * * * *", Timezone: "UTC", RunnerPoolID: "pool-1", DurationSeconds: 60,
	}}
}

func TestProjectionServiceRetainsLastSuccessOnFailure(t *testing.T) {
	var calls atomic.Int32
	repository := projectionRepositoryFunc(func(context.Context) ([]store.ScheduleProjectionInput, error) {
		if calls.Add(1) == 1 {
			return projectionTestInput(), nil
		}
		return nil, errors.New("database unavailable")
	})
	var log bytes.Buffer
	service := NewProjectionService(repository, &platform.Logger{Out: &log})
	if service.Snapshot().Available {
		t.Fatal("snapshot should be unavailable before the first success")
	}
	if err := service.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	want := service.Snapshot()
	if !want.Available || len(want.Segments) == 0 {
		t.Fatalf("snapshot = %#v", want)
	}
	if want.DurationSource != "task_duration" || !strings.Contains(log.String(), `schedule_projection.calculated`) {
		t.Fatalf("metadata/log = %#v / %s", want, log.String())
	}
	if err := service.Refresh(context.Background()); err == nil {
		t.Fatal("expected refresh failure")
	}
	got := service.Snapshot()
	if !got.Available || !got.CalculatedAt.Equal(want.CalculatedAt) || len(got.Segments) != len(want.Segments) {
		t.Fatalf("snapshot changed after failure: got=%#v want=%#v", got, want)
	}
	if !strings.Contains(log.String(), `schedule_projection.calculation_failed`) {
		t.Fatalf("log = %s", log.String())
	}
}

func TestProjectionServiceStartsImmediatelyAndDoesNotOverlap(t *testing.T) {
	var calls, active, maxActive atomic.Int32
	called := make(chan struct{}, 4)
	repository := projectionRepositoryFunc(func(context.Context) ([]store.ScheduleProjectionInput, error) {
		calls.Add(1)
		current := active.Add(1)
		for {
			previous := maxActive.Load()
			if current <= previous || maxActive.CompareAndSwap(previous, current) {
				break
			}
		}
		time.Sleep(2 * time.Millisecond)
		active.Add(-1)
		called <- struct{}{}
		return projectionTestInput(), nil
	})
	service := NewProjectionService(repository, nil)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		service.Run(ctx, time.Millisecond)
		close(done)
	}()
	select {
	case <-called:
	case <-time.After(time.Second):
		t.Fatal("initial refresh did not run")
	}
	select {
	case <-called:
	case <-time.After(time.Second):
		t.Fatal("periodic refresh did not run")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("projection loop did not stop")
	}
	if calls.Load() < 2 || maxActive.Load() != 1 {
		t.Fatalf("calls=%d max_active=%d", calls.Load(), maxActive.Load())
	}
}
