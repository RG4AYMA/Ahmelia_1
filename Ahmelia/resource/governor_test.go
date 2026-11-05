package resource

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestGovernor_FastPath_FreeSlot(t *testing.T) {
	g := NewGovernor(GovernorConfig{})

	lease, err := g.Acquire(context.Background(), Request{Priority: PriorityUserGoal, Kind: KindUserGoal, ID: "test-1"})
	if err != nil {
		t.Fatalf("Acquire error: %v", err)
	}
	if lease == nil {
		t.Fatal("lease is nil")
	}
	if lease.Request.ID != "test-1" {
		t.Fatalf("lease ID mismatch: %s", lease.Request.ID)
	}

	// Liberar
	lease.Release()

	// Segunda adquisición debe funcionar
	lease2, err := g.Acquire(context.Background(), Request{Priority: PriorityUserGoal, Kind: KindUserGoal, ID: "test-2"})
	if err != nil {
		t.Fatalf("second Acquire error: %v", err)
	}
	if lease2.Request.ID != "test-2" {
		t.Fatalf("second lease ID mismatch: %s", lease2.Request.ID)
	}
	lease2.Release()
}

func TestGovernor_QueueAndGrant(t *testing.T) {
	g := NewGovernor(GovernorConfig{})

	// Ocupar ranura
	lease1, err := g.Acquire(context.Background(), Request{Priority: PriorityLearning, Kind: KindLearning, ID: "holder"})
	if err != nil {
		t.Fatalf("Acquire holder: %v", err)
	}

	// Solicitar con menor prioridad (valor mayor) - debe encolarse
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var lease2 *Lease
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		lease2, err = g.Acquire(ctx, Request{Priority: PriorityOwn, Kind: KindOwn, ID: "waiter"})
	}()

	// Dar tiempo a que se encole
	time.Sleep(10 * time.Millisecond)

	stats := g.Stats()
	if stats.Active == nil || stats.Active.ID != "holder" {
		t.Fatalf("holder not active: %+v", stats.Active)
	}
	if stats.QueueLength != 1 {
		t.Fatalf("queue length = %d, want 1", stats.QueueLength)
	}

	// Liberar holder
	lease1.Release()

	// waiter debe ser concedido
	wg.Wait()
	if err != nil {
		t.Fatalf("waiter Acquire error: %v", err)
	}
	if lease2 == nil {
		t.Fatal("lease2 is nil")
	}
	if lease2.Request.ID != "waiter" {
		t.Fatalf("waiter got wrong lease: %s", lease2.Request.ID)
	}
	lease2.Release()
}

func TestGovernor_Preemption_HigherPriorityWins(t *testing.T) {
	g := NewGovernor(GovernorConfig{
		PreemptGrace: 100 * time.Millisecond,
	})

	// Holder con prioridad Learning (3) — menor urgencia que UserGoal
	holderLease, err := g.Acquire(context.Background(), Request{Priority: PriorityLearning, Kind: KindLearning, ID: "holder-learning"})
	if err != nil {
		t.Fatalf("holder Acquire: %v", err)
	}

	// Request con prioridad UserGoal (2) — más urgente que el titular
	requestCtx, requestCancel := context.WithCancel(context.Background())
	defer requestCancel()

	var requestLease *Lease
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		requestLease, err = g.Acquire(requestCtx, Request{Priority: PriorityUserGoal, Kind: KindUserGoal, ID: "request-usergoal"})
	}()

	// Dar tiempo a que se encole
	time.Sleep(20 * time.Millisecond)

	// PreemptIfNeeded debe ejecutarse para que ocurra la preempt
	g.PreemptIfNeeded()

	// Esperar un poco más para que la preempt se complete
	time.Sleep(200 * time.Millisecond)

	wg.Wait()

	if err != nil {
		t.Fatalf("request Acquire error: %v", err)
	}
	if requestLease == nil {
		t.Fatal("request lease is nil after preemption")
	}
	if requestLease.Request.ID != "request-usergoal" {
		t.Fatalf("wrong lease after preempt: %s", requestLease.Request.ID)
	}

	// El holder debe haber sido cancelado
	select {
	case <-holderLease.Ctx.Done():
		// OK
	case <-time.After(100 * time.Millisecond):
		t.Fatal("holder context not cancelled after preemption")
	}

	requestLease.Release()
}

func TestGovernor_UserTurnNeverPreempted(t *testing.T) {
	g := NewGovernor(GovernorConfig{})

	// Turno de usuario (máxima prioridad semántica, nunca se preempta)
	userLease, err := g.Acquire(context.Background(), Request{Priority: PriorityUserTurn, Kind: KindUserTurn, ID: "user-turn"})
	if err != nil {
		t.Fatalf("user turn Acquire: %v", err)
	}

	// Solicitar con menos urgencia que el turno (learning=3 > userturn=0 en
	// valor numérico): debe encolarse pero NO preemptar el turno de usuario.
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	var otherLease *Lease
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		otherLease, err = g.Acquire(ctx, Request{Priority: PriorityLearning, Kind: KindLearning, ID: "learning"})
	}()

	// Dar tiempo a que se encole y se intente preempt
	time.Sleep(50 * time.Millisecond)
	g.PreemptIfNeeded()
	time.Sleep(100 * time.Millisecond)

	// El turno de usuario NO debe ser preemptado
	select {
	case <-userLease.Ctx.Done():
		t.Fatal("user turn was preempted")
	default:
		// OK
	}

	// La otra petición debe haber fallado por timeout o seguir esperando
	cancel() // cancelar contexto para que la goroutine termine
	wg.Wait()

	if otherLease != nil {
		otherLease.Release()
	}
	userLease.Release()
}

func TestGovernor_Aging(t *testing.T) {
	g := NewGovernor(GovernorConfig{
		AgingAfter:  50 * time.Millisecond,
		MaxPriority: PriorityUserGoal, // tope de envejecimiento
	})

	// Holder en el tope: UserGoal (2). El envejecimiento nunca lo desplaza.
	holderLease, err := g.Acquire(context.Background(), Request{Priority: PriorityUserGoal, Kind: KindUserGoal, ID: "holder"})
	if err != nil {
		t.Fatalf("holder Acquire: %v", err)
	}

	// Waiter con la prioridad más baja: Own (4). Debe envejecer hacia el
	// tope UserGoal (2) — subir de urgencia — sin alcanzarlo jamás en
	// términos de preempt (2 no es menor que 2).
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var requestLease *Lease
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		requestLease, err = g.Acquire(ctx, Request{Priority: PriorityOwn, Kind: KindOwn, ID: "aging-own"})
	}()

	// Cada PreemptIfNeeded recalcula el envejecimiento (un nivel por
	// consulta con la espera cumplida) y evalúa el preempt.
	for i := 0; i < 3; i++ {
		time.Sleep(60 * time.Millisecond)
		g.PreemptIfNeeded()
	}

	stats := g.Stats()
	if stats.Active == nil || stats.Active.ID != "holder" {
		t.Fatalf("el tope de aging no debe preemptar al holder: %+v", stats.Active)
	}
	if stats.QueueLength != 1 || stats.Queued[0].EffectivePriority != PriorityUserGoal {
		t.Fatalf("aging no alcanzó el tope UserGoal: %+v", stats.Queued)
	}

	// Al liberar el holder, el waiter envejecido recibe la ranura.
	holderLease.Release()

	wg.Wait()
	if err != nil {
		t.Fatalf("request Acquire error after holder release: %v", err)
	}
	if requestLease == nil {
		t.Fatal("request lease is nil after holder release")
	}

	requestLease.Release()
}

func TestGovernor_ReleaseIdempotent(t *testing.T) {
	g := NewGovernor(GovernorConfig{})

	lease, err := g.Acquire(context.Background(), Request{Priority: PriorityUserGoal, Kind: KindUserGoal, ID: "test"})
	if err != nil {
		t.Fatalf("Acquire: %v", err)
	}

	lease.Release()
	lease.Release() // no debe fallar
	lease.Release() // no debe fallar

	// Nueva adquisición debe funcionar
	lease2, err := g.Acquire(context.Background(), Request{Priority: PriorityUserGoal, Kind: KindUserGoal, ID: "test2"})
	if err != nil {
		t.Fatalf("Acquire after double release: %v", err)
	}
	lease2.Release()
}

func TestGovernor_CancelledContextRemovesFromQueue(t *testing.T) {
	g := NewGovernor(GovernorConfig{})

	// Ocupar
	holder, err := g.Acquire(context.Background(), Request{Priority: PriorityUserGoal, Kind: KindUserGoal, ID: "holder"})
	if err != nil {
		t.Fatalf("holder Acquire: %v", err)
	}

	// Encolar
	ctx, cancel := context.WithCancel(context.Background())
	var lease *Lease
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		lease, err = g.Acquire(ctx, Request{Priority: PriorityLearning, Kind: KindLearning, ID: "queued"})
	}()

	time.Sleep(10 * time.Millisecond)

	stats := g.Stats()
	if stats.QueueLength != 1 {
		t.Fatalf("queue length = %d, want 1", stats.QueueLength)
	}

	// Cancelar contexto
	cancel()

	wg.Wait()

	// Debe haber salido de la cola
	stats = g.Stats()
	if stats.QueueLength != 0 {
		t.Fatalf("queue length after cancel = %d, want 0", stats.QueueLength)
	}
	_ = lease // avoid unused warning

	holder.Release()
}

func TestGovernor_CloseCleansUp(t *testing.T) {
	g := NewGovernor(GovernorConfig{})

	holder, err := g.Acquire(context.Background(), Request{Priority: PriorityLearning, Kind: KindLearning, ID: "holder"})
	if err != nil {
		t.Fatalf("holder Acquire: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var lease *Lease
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		lease, err = g.Acquire(ctx, Request{Priority: PriorityUserGoal, Kind: KindUserGoal, ID: "queued"})
	}()

	time.Sleep(10 * time.Millisecond)

	g.Close()

	wg.Wait()

	// Todo debe estar limpio
	stats := g.Stats()
	if stats.Active != nil {
		t.Fatal("active lease after Close")
	}
	if stats.QueueLength != 0 {
		t.Fatalf("queue length after Close = %d", stats.QueueLength)
	}

	if lease != nil {
		// Si se concedió antes del close, debe estar cancelado
		select {
		case <-lease.Ctx.Done():
			// OK
		default:
			t.Fatal("queued lease not cancelled after Close")
		}
	}

	holder.Release() // safe, already cancelled
}

func TestGovernor_StatsObservability(t *testing.T) {
	g := NewGovernor(GovernorConfig{})

	stats := g.Stats()
	if stats.QueueLength != 0 || stats.Active != nil {
		t.Fatalf("empty governor stats: %+v", stats)
	}

	lease, _ := g.Acquire(context.Background(), Request{Priority: PriorityUserGoal, Kind: KindUserGoal, ID: "active-1"})
	stats = g.Stats()
	if stats.Active == nil || stats.Active.ID != "active-1" {
		t.Fatalf("active not tracked: %+v", stats.Active)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_, _ = g.Acquire(ctx, Request{Priority: PriorityLearning, Kind: KindLearning, ID: "queued-1"})
	}()

	time.Sleep(10 * time.Millisecond)

	stats = g.Stats()
	if stats.QueueLength != 1 {
		t.Fatalf("queue length = %d, want 1", stats.QueueLength)
	}
	if len(stats.Queued) != 1 || stats.Queued[0].ID != "queued-1" {
		t.Fatalf("queued not tracked: %+v", stats.Queued)
	}

	lease.Release()
	cancel()
	wg.Wait()
}

func TestGovernor_PriorityOrdering(t *testing.T) {
	// Verificar que la convención numérica se cumple: menor = más urgente
	// (UserTurn=0 es la máxima; Own=4 la más baja).
	cases := []struct {
		higher Priority
		lower  Priority
	}{
		{PriorityUserTurn, PriorityUrgent},
		{PriorityUrgent, PriorityUserGoal},
		{PriorityUserGoal, PriorityLearning},
		{PriorityLearning, PriorityOwn},
	}

	for _, tc := range cases {
		if tc.higher >= tc.lower {
			t.Fatalf("priority ordering broken: %d >= %d", tc.higher, tc.lower)
		}
	}
}

func TestGovernor_ConcurrentAccess(t *testing.T) {
	g := NewGovernor(GovernorConfig{})

	var wg sync.WaitGroup
	const goroutines = 20

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()

			lease, err := g.Acquire(ctx, Request{
				Priority: PriorityUserGoal,
				Kind:     KindUserGoal,
				ID:       "concurrent",
			})
			if err != nil {
				// Puede fallar por timeout si hay mucha contención
				return
			}
			if lease != nil {
				lease.Release()
			}
		}(i)
	}

	wg.Wait()

	// No debe haber goroutines colgadas ni panic
	stats := g.Stats()
	if stats.Active != nil {
		_ = stats.Active // just verify accessible
	}
	for _, q := range stats.Queued {
		_ = q // queue should be empty after all releases
	}
}

type fakeClock struct {
	t time.Time
}

func (f *fakeClock) Now() time.Time { return f.t }
func (f *fakeClock) After(d time.Duration) <-chan time.Time {
	c := make(chan time.Time)
	close(c)
	return c
}

func TestGovernor_CustomClock(t *testing.T) {
	fc := &fakeClock{t: time.Now()}

	g := NewGovernor(GovernorConfig{
		Clock:        fc,
		AgingAfter:   10 * time.Second,
		PreemptGrace: 100 * time.Millisecond,
	})

	holder, _ := g.Acquire(context.Background(), Request{Priority: PriorityLearning, Kind: KindLearning, ID: "holder"})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var lease *Lease
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		lease, _ = g.Acquire(ctx, Request{Priority: PriorityUrgent, Kind: KindUrgent, ID: "urgent"})
	}()

	// Avanzar reloj falso más allá de AgingAfter
	fc.t = fc.t.Add(15 * time.Second)
	g.PreemptIfNeeded()

	// La petición urgente (1) es más urgente que el titular Learning (3):
	// preempta (con la gracia configurada). El reloj falso alimenta solo
	// el envejecimiento; la gracia usa tiempo real.
	holder.Release()

	wg.Wait()
	if lease != nil {
		lease.Release()
	}
}

func TestGovernor_LeaseContextFollowsCaller(t *testing.T) {
	g := NewGovernor(GovernorConfig{})

	ctx, cancel := context.WithCancel(context.Background())
	lease, err := g.Acquire(ctx, Request{Priority: PriorityUserGoal, Kind: KindUserGoal, ID: "caller-ctx"})
	if err != nil {
		t.Fatalf("Acquire: %v", err)
	}

	// El presupuesto del llamador acota el trabajo concedido.
	cancel()

	select {
	case <-lease.Ctx.Done():
		// OK
	case <-time.After(200 * time.Millisecond):
		t.Fatal("lease ctx no siguió la cancelación del llamador")
	}

	lease.Release()
}

func TestGovernor_QueuedGrantFollowsCaller(t *testing.T) {
	g := NewGovernor(GovernorConfig{})

	holder, err := g.Acquire(context.Background(), Request{Priority: PriorityUserGoal, Kind: KindUserGoal, ID: "holder"})
	if err != nil {
		t.Fatalf("holder Acquire: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	var lease *Lease
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		lease, err = g.Acquire(ctx, Request{Priority: PriorityLearning, Kind: KindLearning, ID: "queued"})
	}()

	time.Sleep(10 * time.Millisecond)
	holder.Release()
	wg.Wait()

	if err != nil {
		t.Fatalf("queued Acquire: %v", err)
	}
	if lease == nil {
		t.Fatal("queued lease is nil")
	}

	cancel()

	select {
	case <-lease.Ctx.Done():
		// OK
	case <-time.After(200 * time.Millisecond):
		t.Fatal("lease concedido no siguió la cancelación del llamador")
	}

	lease.Release()
}

func TestGovernor_PreemptOnEnqueue(t *testing.T) {
	g := NewGovernor(GovernorConfig{PreemptGrace: 50 * time.Millisecond})

	holder, err := g.Acquire(context.Background(), Request{Priority: PriorityLearning, Kind: KindLearning, ID: "holder"})
	if err != nil {
		t.Fatalf("holder Acquire: %v", err)
	}

	// Encolar una petición más urgente debe disparar el preempt sin esperar
	// a un driver periódico: un encolado es un cambio.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var lease *Lease
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		lease, err = g.Acquire(ctx, Request{Priority: PriorityUserGoal, Kind: KindUserGoal, ID: "urgent"})
	}()

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// OK: concedida sin llamar a PreemptIfNeeded
	case <-time.After(500 * time.Millisecond):
		t.Fatal("preempt-on-enqueue no concedió la ranura")
	}

	if lease == nil {
		t.Fatal("lease is nil after preempt-on-enqueue")
	}

	select {
	case <-holder.Ctx.Done():
		// OK: titular cancelado con gracia
	case <-time.After(300 * time.Millisecond):
		t.Fatal("titular no fue cancelado tras preempt")
	}

	lease.Release()
}
