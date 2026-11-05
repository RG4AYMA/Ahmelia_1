package linguistics

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode"

	"ahmelia/interaction"
	"ahmelia/shared"
)

// DeferredJob is a delayed linguistic interpretation request. It contains the
// minimum semantic/context snapshot needed to reinterpret the original turn.
// It does not contain decision, response, capability, or execution authority.
type DeferredJob struct {
	ID         string
	Request    interaction.SemanticRequest
	Context    interaction.Context
	EnqueuedAt time.Time
	Signature  string
}

// DeferredResult is evidence produced by the same Interpreter used by the
// live path. The result remains reusable through its semantic signature even
// after the corresponding initiative candidate has been delivered.
type DeferredResult struct {
	Job            DeferredJob
	Interpretation Interpretation
	// Open carries the open comprehension contract result when the resolver
	// runs in open mode; Interpretation stays zero then.
	Open        *OpenMeaning
	StartedAt   time.Time
	CompletedAt time.Time
	Error       string
	Ready       bool
	Delivered   bool
}

// DeferredObserver is diagnostic/evidence-only. Implementations must never
// answer the user or invoke a decision authority.
type DeferredObserver func(DeferredResult)

// DeferredResolver owns the queue, one worker, and an in-memory result/cache
// store. It is a timing/infrastructure boundary around the same linguistic
// Interpreter, not a second cognitive system.
type DeferredResolver struct {
	interpreter *Interpreter
	queue       chan DeferredJob

	mu             sync.RWMutex
	jobs           map[string]DeferredJob
	results        map[string]DeferredResult
	bySignature    map[string]string
	pendingSignats map[string]string

	observer          DeferredObserver
	resolveTimeout    time.Duration
	ctx               context.Context
	cancel            context.CancelFunc
	activeCancel      context.CancelFunc
	wg                sync.WaitGroup
	started           atomic.Bool
	counter           atomic.Uint64
	openComprehension bool
}

// NewDeferredResolver creates the single deferred linguistic infrastructure
// instance for one Engine. The queue is intentionally bounded so a slow local
// model cannot grow an unbounded memory backlog.
func NewDeferredResolver(interpreter *Interpreter, capacity int) *DeferredResolver {
	return NewDeferredResolverWithTimeout(interpreter, capacity, deferredResolverTimeout())
}

func deferredResolverTimeout() time.Duration {
	value := strings.TrimSpace(os.Getenv("AHMELIA_LINGUISTIC_DEFERRED_TIMEOUT_MS"))
	if value == "" {
		return 120 * time.Second
	}
	ms, err := strconv.Atoi(value)
	if err != nil || ms < 1000 {
		return 120 * time.Second
	}
	if ms > 600000 {
		ms = 600000
	}
	return time.Duration(ms) * time.Millisecond
}

func NewDeferredResolverWithTimeout(interpreter *Interpreter, capacity int, timeout time.Duration) *DeferredResolver {
	if capacity <= 0 {
		capacity = 16
	}
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	return &DeferredResolver{
		interpreter:    interpreter,
		queue:          make(chan DeferredJob, capacity),
		jobs:           make(map[string]DeferredJob),
		results:        make(map[string]DeferredResult),
		bySignature:    make(map[string]string),
		pendingSignats: make(map[string]string),
		resolveTimeout: timeout,
	}
}

func (r *DeferredResolver) SetObserver(observer DeferredObserver) {
	if r == nil {
		return
	}
	r.mu.Lock()
	r.observer = observer
	r.mu.Unlock()
}

// SetOpenComprehension switches the deferred worker from the closed
// interpretation contract to the open meaning contract (owner directive
// 2026-10-17 16:04). Only the live composition root enables it; suites keep
// the closed contract by default.
func (r *DeferredResolver) SetOpenComprehension(enabled bool) {
	if r == nil {
		return
	}
	r.openComprehension = enabled
}

// Start starts exactly one worker. Constructing the resolver does not start a
// goroutine; Engine startup owns the lifecycle explicitly, consistent with the
// project's no-side-effects NewEngine rule.
func (r *DeferredResolver) Start(ctx context.Context) {
	if r == nil || r.interpreter == nil || r.started.Swap(true) {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}
	r.ctx, r.cancel = context.WithCancel(ctx)
	r.wg.Add(1)
	go r.worker()
}

func (r *DeferredResolver) worker() {
	defer r.wg.Done()
	for {
		select {
		case <-r.ctx.Done():
			return
		case job := <-r.queue:
			r.resolve(job)
		}
	}
}

func (r *DeferredResolver) resolve(job DeferredJob) {
	started := time.Now()
	result := DeferredResult{Job: job, StartedAt: started}
	if r.interpreter == nil {
		result.Error = "interpreter no configurado"
	} else {
		resolveCtx := r.ctx
		var cancel context.CancelFunc
		if r.resolveTimeout > 0 {
			resolveCtx, cancel = context.WithTimeout(r.ctx, r.resolveTimeout)
		} else {
			resolveCtx, cancel = context.WithCancel(r.ctx)
		}
		r.mu.Lock()
		r.activeCancel = cancel
		r.mu.Unlock()
		defer func() {
			cancel()
			r.mu.Lock()
			r.activeCancel = nil
			r.mu.Unlock()
		}()
		if r.openComprehension {
			meaning, err := r.interpreter.InterpretOpenContext(resolveCtx, job.Request.Raw)
			if err != nil {
				result.Error = err.Error()
			} else {
				result.Open = &meaning
				result.Ready = true
				result.Job.Signature = resolvedSemanticSignatureOpen(job.Request, meaning)
			}
		} else {
			interpretation, err := r.interpreter.InterpretContext(resolveCtx, job.Request.Raw)
			if err != nil {
				result.Error = err.Error()
			} else {
				result.Interpretation = interpretation
				result.Ready = true
				result.Job.Signature = resolvedSemanticSignature(job.Request, interpretation)
			}
		}
	}
	result.CompletedAt = time.Now()

	r.mu.Lock()
	delete(r.pendingSignats, job.Signature)
	if result.Ready {
		r.results[job.ID] = result
		r.bySignature[result.Job.Signature] = job.ID
		// Keep the original routing signature as an alias too. This allows a
		// second formulation to reuse a result even when the resolved model
		// adds a more specific topic signature.
		if job.Signature != result.Job.Signature {
			r.bySignature[job.Signature] = job.ID
		}
	}
	observer := r.observer
	r.mu.Unlock()

	if observer != nil {
		observer(result)
	}
}

// CancelOptionalInference gives higher-priority work a way to preempt
// linguistic enrichment that is no longer useful to the current turn.
//
// Deferred interpretation is evidence, not authority. Once the canonical
// interaction path is ready to execute work, keeping a long-running optional
// model request alive can compete with the same local model that execution
// needs. Cancelling the optional request is therefore a scheduling decision,
// not an error path and does not stop the resolver itself.
func (r *DeferredResolver) CancelOptionalInference() {
	if r == nil {
		return
	}
	r.mu.Lock()
	cancel := r.activeCancel
	for id := range r.jobs {
		delete(r.jobs, id)
	}
	for signature := range r.pendingSignats {
		delete(r.pendingSignats, signature)
	}
	r.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

// Submit queues a semantic turn for deferred interpretation. Duplicate work
// with the same semantic signature is coalesced instead of spawning another
// model inference.
func (r *DeferredResolver) Submit(req interaction.SemanticRequest, ctxSnapshot interaction.Context) (string, error) {
	if r == nil {
		return "", fmt.Errorf("deferred linguistic resolver unavailable")
	}
	if r.interpreter == nil {
		return "", fmt.Errorf("deferred linguistic interpreter unavailable")
	}
	if !r.started.Load() {
		return "", fmt.Errorf("deferred linguistic resolver not started")
	}
	if strings.TrimSpace(req.Raw) == "" {
		return "", fmt.Errorf("deferred linguistic request vacío")
	}
	signature := SemanticSignatureForRequest(req)
	if signature == "" {
		return "", fmt.Errorf("deferred linguistic signature vacío")
	}

	r.mu.Lock()
	if existing, ok := r.pendingSignats[signature]; ok {
		r.mu.Unlock()
		return existing, nil
	}
	jobID := r.nextJobID()
	job := DeferredJob{ID: jobID, Request: req, Context: ctxSnapshot, EnqueuedAt: time.Now(), Signature: signature}
	r.jobs[jobID] = job
	r.pendingSignats[signature] = jobID
	r.mu.Unlock()

	select {
	case r.queue <- job:
		return jobID, nil
	case <-r.ctxDone():
		r.mu.Lock()
		delete(r.jobs, jobID)
		delete(r.pendingSignats, signature)
		r.mu.Unlock()
		return "", fmt.Errorf("deferred linguistic resolver stopped")
	default:
		r.mu.Lock()
		delete(r.jobs, jobID)
		delete(r.pendingSignats, signature)
		r.mu.Unlock()
		return "", fmt.Errorf("deferred linguistic queue full")
	}
}

func (r *DeferredResolver) ctxDone() <-chan struct{} {
	if r == nil || r.ctx == nil {
		return nil
	}
	return r.ctx.Done()
}

func (r *DeferredResolver) nextJobID() string {
	n := r.counter.Add(1)
	return fmt.Sprintf("ling-deferred-%d-%d", time.Now().UnixNano(), n)
}

// Lookup returns a completed interpretation reusable by semantic signature.
// It is cache access only; it never marks an initiative as delivered.
func (r *DeferredResolver) Lookup(req interaction.SemanticRequest) (DeferredResult, bool) {
	if r == nil {
		return DeferredResult{}, false
	}
	sig := SemanticSignatureForRequest(req)
	if sig == "" {
		return DeferredResult{}, false
	}
	r.mu.RLock()
	jobID, ok := r.bySignature[sig]
	result, ready := r.results[jobID]
	r.mu.RUnlock()
	return result, ok && ready && result.Ready && result.Error == ""
}

func (r *DeferredResolver) Get(jobID string) (DeferredResult, bool) {
	if r == nil || strings.TrimSpace(jobID) == "" {
		return DeferredResult{}, false
	}
	r.mu.RLock()
	result, ok := r.results[jobID]
	r.mu.RUnlock()
	return result, ok && result.Ready && result.Error == ""
}

// AwaitJob waits, bounded by ctx, for one specific deferred job to reach a
// terminal state. It is the in-turn counterpart of the reuse cache: a live
// turn whose rules classification produced no typed act may wait for the very
// reading it submitted, with an explicit caller budget, instead of answering
// from an empty surface. Returns the completed result when ready; false on
// error, cancellation or budget exhaustion — a failed reading is never an
// error of the turn.
func (r *DeferredResolver) AwaitJob(ctx context.Context, jobID string) (DeferredResult, bool) {
	if r == nil || strings.TrimSpace(jobID) == "" {
		return DeferredResult{}, false
	}
	if result, ok := r.Get(jobID); ok {
		return result, true
	}
	if ctx == nil {
		ctx = context.Background()
	}
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return DeferredResult{}, false
		case <-ticker.C:
			if result, ok := r.Get(jobID); ok {
				return result, true
			}
			// A job that no longer holds a pending signature finished without
			// a reusable result (error, cancellation or preemption): waiting
			// longer cannot change the outcome.
			r.mu.RLock()
			job, tracked := r.jobs[jobID]
			pendingID, pending := r.pendingSignats[job.Signature]
			r.mu.RUnlock()
			if !tracked || !pending || pendingID != jobID {
				return DeferredResult{}, false
			}
		}
	}
}

// Ready returns completed results not yet delivered through the initiative
// path. Results remain in the cache after delivery.
func (r *DeferredResolver) Ready() []DeferredResult {
	if r == nil {
		return nil
	}
	r.mu.RLock()
	out := make([]DeferredResult, 0, len(r.results))
	for _, result := range r.results {
		if result.Ready && result.Error == "" && !result.Delivered {
			out = append(out, result)
		}
	}
	r.mu.RUnlock()
	sort.SliceStable(out, func(i, j int) bool { return out[i].CompletedAt.Before(out[j].CompletedAt) })
	return out
}

func (r *DeferredResolver) MarkDelivered(jobID string) {
	if r == nil || strings.TrimSpace(jobID) == "" {
		return
	}
	r.mu.Lock()
	if result, ok := r.results[jobID]; ok {
		result.Delivered = true
		r.results[jobID] = result
	}
	r.mu.Unlock()
}

// Stop cancels the single worker and waits for it to finish. The worker is the
// only component that calls the Interpreter asynchronously; no response path
// is reachable from this package.
func (r *DeferredResolver) Stop() {
	if r == nil || !r.started.Load() {
		return
	}
	if r.cancel != nil {
		r.cancel()
	}
	r.wg.Wait()
	r.started.Store(false)
}

// SemanticSignatureForRequest derives a reusable structural key from the
// semantic request. It deliberately does not store the literal sentence.
// The fallback content component removes generic function words and ownership
// markers while retaining content-bearing terms, allowing different surface
// constructions to share a cache key without introducing a phrase template.
func SemanticSignatureForRequest(req interaction.SemanticRequest) string {
	kind := string(req.Kind)
	if kind == "" {
		kind = "unknown"
	}
	owner := string(req.Subject)
	if owner == "" {
		owner = "unknown"
	}
	qtype := strings.ToLower(strings.TrimSpace(shared.Normalize(req.QuestionType)))
	if qtype == "" {
		qtype = "general"
	}
	topic := normalizeSignatureContent(req.Topic)
	if topic == "" {
		topic = contentSignature(req.Raw)
	}
	if topic == "" {
		topic = contentSignature(strings.Join(req.Meaning.Evidence, " "))
	}
	if topic == "" {
		return ""
	}
	return strings.Join([]string{kind, owner, qtype, topic}, "|")
}

func resolvedSemanticSignature(req interaction.SemanticRequest, interpretation Interpretation) string {
	candidate := req
	if strings.TrimSpace(interpretation.Topic) != "" {
		candidate.Topic = interpretation.Topic
	}
	if interpretation.Intent != "" && candidate.Kind == interaction.KindUnknown {
		candidate.Kind = interactionKindFromIntent(interpretation.Intent)
	}
	return SemanticSignatureForRequest(candidate)
}

func interactionKindFromIntent(intent any) interaction.RequestKind {
	value := fmt.Sprint(intent)
	switch value {
	case "question":
		return interaction.KindQuestion
	case "feedback":
		return interaction.KindFeedback
	case "correction":
		return interaction.KindCorrection
	case "learning":
		return interaction.KindLearning
	case "command":
		return interaction.KindAction
	default:
		return interaction.KindUnknown
	}
}

// resolvedSemanticSignatureOpen refines the routing signature with the open
// comprehension result so a second formulation of the same meaning can reuse
// this evidence even when the model adds a more specific topic signature.
func resolvedSemanticSignatureOpen(req interaction.SemanticRequest, meaning OpenMeaning) string {
	candidate := req
	if strings.TrimSpace(meaning.Topic) != "" {
		candidate.Topic = strings.TrimSpace(meaning.Topic)
	}
	if candidate.Kind == interaction.KindUnknown {
		switch meaning.Purpose {
		case OpenPurposeQuestion:
			candidate.Kind = interaction.KindQuestion
		case OpenPurposeFeedback:
			candidate.Kind = interaction.KindFeedback
		case OpenPurposeRequest:
			candidate.Kind = interaction.KindAction
		}
	}
	return SemanticSignatureForRequest(candidate)
}

func normalizeSignatureContent(value string) string {
	return contentSignature(value)
}

func contentSignature(value string) string {
	value = strings.TrimSpace(shared.Normalize(value))
	if value == "" {
		return ""
	}
	stop := signatureStopwords()
	seen := make(map[string]struct{})
	words := make([]string, 0, 8)
	for _, raw := range strings.FieldsFunc(value, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsNumber(r) }) {
		w := strings.TrimSpace(raw)
		if len([]rune(w)) < 2 {
			continue
		}
		if _, blocked := stop[w]; blocked {
			continue
		}
		if _, exists := seen[w]; exists {
			continue
		}
		seen[w] = struct{}{}
		words = append(words, w)
	}
	return strings.Join(words, " ")
}

func signatureStopwords() map[string]struct{} {
	items := []string{
		"a", "al", "algo", "alguna", "alguno", "algun", "alguna", "alguno",
		"como", "cual", "cuales", "cuando", "de", "del", "dime", "donde",
		"el", "ella", "ellos", "en", "es", "esa", "ese", "esto", "esta",
		"estas", "este", "hay", "idea", "las", "lo", "los", "me", "mi", "mis",
		"para", "por", "que", "quien", "recuerdas", "sabes", "se", "si", "su",
		"tengo", "tienes", "tenes", "tener", "tu", "tus", "una", "unas", "uno",
		"usted", "ustedes", "yo", "conoces", "conocer", "conocimiento",
		"presente", "favor", "favorita", "favorito", "mas", "podrias",
	}
	out := make(map[string]struct{}, len(items))
	for _, item := range items {
		out[strings.TrimSpace(shared.Normalize(item))] = struct{}{}
	}
	// Keep domain-bearing variants that frequently appear as part of a
	// semantic topic. "favorito" and "favorita" are intentionally retained by
	// removing them from the stop set below.
	delete(out, "favorito")
	delete(out, "favorita")
	return out
}
