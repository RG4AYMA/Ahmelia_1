package programming

import (
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"
)

// B96.10 — Runtime Execution
//
// Runtime execution starts a built program and exposes its lifecycle without
// becoming an authority for safety, authorization, observation or recovery.
// Those concerns remain owned by B95/B94.

type RuntimeRequest struct {
	ID         string
	Project    CodeProject
	Command    string
	WorkingDir string
	Timeout    time.Duration
}

func (r RuntimeRequest) Validate() error {
	if r.ID == "" {
		return fmt.Errorf("programming runtime request: ID vacío")
	}
	if err := r.Project.Validate(); err != nil {
		return err
	}
	if strings.TrimSpace(r.Command) == "" {
		return fmt.Errorf("programming runtime request %q: Command vacío", r.ID)
	}
	wd := r.WorkingDir
	if wd == "" {
		wd = r.Project.Root
	}
	if !pathWithin(wd, r.Project.Root) {
		return fmt.Errorf("programming runtime request %q: working directory fuera del proyecto", r.ID)
	}
	return nil
}

type RuntimeExecutor interface {
	Start(context.Context, RuntimeRequest) (Runtime, error)
	Stop(string) (Runtime, error)
	Get(string) (Runtime, bool)
}

type runtimeOutput struct {
	mu   sync.Mutex
	text strings.Builder
}

func (o *runtimeOutput) Write(p []byte) (int, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.text.Write(p)
}

func (o *runtimeOutput) String() string {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.text.String()
}

type runtimeProcess struct {
	cmd       *exec.Cmd
	output    *runtimeOutput
	startedAt time.Time
	runtime   Runtime
	cancel    context.CancelFunc
	live      bool
}

type OSRuntimeExecutor struct {
	mu      sync.Mutex
	process map[string]*runtimeProcess
}

func NewOSRuntimeExecutor() *OSRuntimeExecutor {
	return &OSRuntimeExecutor{process: make(map[string]*runtimeProcess)}
}

func (e *OSRuntimeExecutor) Start(ctx context.Context, req RuntimeRequest) (Runtime, error) {
	if err := req.Validate(); err != nil {
		return Runtime{ID: req.ID, ProjectID: req.Project.ID, Command: req.Command, Status: RuntimeFailed}, err
	}
	if ctx == nil {
		ctx = context.Background()
	}

	wd := req.WorkingDir
	if wd == "" {
		wd = req.Project.Root
	}

	e.mu.Lock()
	if existing, exists := e.process[req.ID]; exists && existing.live {
		e.mu.Unlock()
		return Runtime{ID: req.ID, ProjectID: req.Project.ID, Command: req.Command, Status: RuntimeFailed}, fmt.Errorf("programming runtime %q: ya está ejecutándose", req.ID)
	}

	runCtx := ctx
	cancel := func() {}
	if req.Timeout > 0 {
		runCtx, cancel = context.WithTimeout(ctx, req.Timeout)
	}
	cmd := runtimeShellCommand(runCtx, req.Command)
	cmd.Dir = wd
	output := &runtimeOutput{}
	// Attach the synchronized output sink directly to the command. This keeps
	// stdout/stderr capture coupled to Cmd.Wait's lifecycle and avoids a
	// separate reader race where a completed runtime could briefly expose an
	// empty output buffer. runtimeOutput itself serializes concurrent stdout
	// and stderr writes.
	cmd.Stdout = output
	cmd.Stderr = output

	started := time.Now().UTC()
	if err := cmd.Start(); err != nil {
		cancel()
		e.mu.Unlock()
		return Runtime{ID: req.ID, ProjectID: req.Project.ID, Command: req.Command, Status: RuntimeFailed, StartedAt: started, StoppedAt: time.Now().UTC()}, err
	}

	r := Runtime{ID: req.ID, ProjectID: req.Project.ID, Command: req.Command, Status: RuntimeRunning, StartedAt: started}
	p := &runtimeProcess{cmd: cmd, output: output, startedAt: started, runtime: r, cancel: cancel, live: true}
	e.process[req.ID] = p
	e.mu.Unlock()

	go e.wait(req.ID, p)
	return r, nil
}

func (e *OSRuntimeExecutor) wait(id string, p *runtimeProcess) {
	err := p.cmd.Wait()
	stopped := time.Now().UTC()

	e.mu.Lock()
	defer e.mu.Unlock()
	current, ok := e.process[id]
	if !ok || current != p {
		p.cancel()
		return
	}

	r := p.runtime
	r.Output = p.output.String()
	r.StoppedAt = stopped
	if err == nil {
		r.Status = RuntimeStopped
		exit := 0
		r.ExitCode = &exit
	} else {
		r.Status = RuntimeFailed
		if exitErr, ok := err.(*exec.ExitError); ok {
			code := exitErr.ExitCode()
			r.ExitCode = &code
		}
	}
	p.runtime = r
	p.live = false
	p.cancel()
}

func (e *OSRuntimeExecutor) Stop(id string) (Runtime, error) {
	if strings.TrimSpace(id) == "" {
		return Runtime{}, fmt.Errorf("programming runtime: ID vacío")
	}
	e.mu.Lock()
	p, ok := e.process[id]
	if !ok {
		e.mu.Unlock()
		return Runtime{}, fmt.Errorf("programming runtime %q: no encontrado", id)
	}
	if !p.live || p.cmd == nil || p.cmd.Process == nil {
		r := p.runtime
		e.mu.Unlock()
		return r, nil
	}
	if err := p.cmd.Process.Kill(); err != nil {
		e.mu.Unlock()
		return p.runtime, err
	}
	r := p.runtime
	e.mu.Unlock()
	return r, nil
}

func (e *OSRuntimeExecutor) Get(id string) (Runtime, bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	p, ok := e.process[id]
	if !ok {
		return Runtime{}, false
	}
	r := p.runtime
	r.Output = p.output.String()
	return r, true
}

func runtimeShellCommand(ctx context.Context, command string) *exec.Cmd {
	if runtime.GOOS == "windows" {
		return exec.CommandContext(ctx, "cmd", "/C", command)
	}
	return exec.CommandContext(ctx, "sh", "-c", command)
}
