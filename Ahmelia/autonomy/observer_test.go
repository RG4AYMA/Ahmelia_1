package autonomy

import (
	"sync"
	"testing"
	"time"
)

func TestContextObserver_ObserveAndBuffer(t *testing.T) {
	cfg := ObserverConfig{
		MaxBufferSize:       3,
		DeduplicateWindowMs: 500 * time.Millisecond,
	}
	obs := NewContextObserver(cfg)

	p1 := map[string]interface{}{"file": "main.go"}
	o1 := obs.Observe(TypeFileChanged, "VSCode", p1)

	if o1 == nil {
		t.Fatalf("se esperaba una observación válida, se obtuvo nil")
	}

	if o1.Processed {
		t.Errorf("la observación debe crearse como no procesada")
	}

	unproc := obs.GetUnprocessedObservations()
	if len(unproc) != 1 {
		t.Errorf("se esperaba 1 observación no procesada, se obtuvo %d", len(unproc))
	}
}

func TestContextObserver_Deduplication(t *testing.T) {
	cfg := ObserverConfig{
		MaxBufferSize:       10,
		DeduplicateWindowMs: 200 * time.Millisecond,
	}
	obs := NewContextObserver(cfg)

	payload := map[string]interface{}{"app": "Unity.exe"}

	o1 := obs.Observe(TypeAppFocused, "OS", payload)
	o2 := obs.Observe(TypeAppFocused, "OS", payload)

	if o1 == nil {
		t.Fatalf("la primera observación no debió ser nula")
	}
	if o2 != nil {
		t.Errorf("la segunda observación debió ser descartada por deduplicación")
	}

	time.Sleep(250 * time.Millisecond)
	o3 := obs.Observe(TypeAppFocused, "OS", payload)

	if o3 == nil {
		t.Errorf("la tercera observación debió aceptarse tras expiración del margen")
	}
}

func TestContextObserver_SubscriptionNotification(t *testing.T) {
	obs := NewContextObserver(ObserverConfig{})

	var wg sync.WaitGroup
	wg.Add(1)

	var received ContextObservation

	obs.Subscribe(func(o ContextObservation) {
		received = o
		wg.Done()
	})

	obs.Observe(TypeTimerElapsed, "Cron", map[string]interface{}{"id": "tick_30m"})

	wg.Wait()

	if received.Source != "Cron" || received.Type != TypeTimerElapsed {
		t.Errorf("notificación de listener incorrecta")
	}
}
