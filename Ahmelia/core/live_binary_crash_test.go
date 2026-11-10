package core

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"ahmelia/production"
	"ahmelia/programming"
)

// TestLiveBinaryCrashRecovery compiles the real Ahmelia binary, runs it with
// Ollama in CPU, executes a programming effect, kills the whole process tree
// while the project tests are still running (after the files were written and
// before the provider result is finalized), and restarts the same binary to
// reconcile from the durable WorkStore.
//
// Opt-in: AHMELIA_LIVE_BINARY=1. It does not use the test binary as the
// application; it starts the compiled main package.
func TestLiveBinaryCrashRecovery(t *testing.T) {
	if os.Getenv("AHMELIA_LIVE_BINARY") != "1" {
		t.Skip("definir AHMELIA_LIVE_BINARY=1 para compilar y matar el binario real")
	}
	if v := strings.TrimSpace(os.Getenv("AHMELIA_OLLAMA_NUM_GPU")); v != "" {
		t.Fatalf("esta prueba es CPU-only; quitar AHMELIA_OLLAMA_NUM_GPU (valor=%q)", v)
	}

	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller no disponible")
	}
	repoRoot := filepath.Dir(filepath.Dir(thisFile))
	workRoot := t.TempDir()
	bin := filepath.Join(workRoot, "ahmelia_crash.exe")
	build := exec.Command("go", "build", "-o", bin, ".")
	build.Dir = repoRoot
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("compilar binario: %v\n%s", err, out)
	}

	runDir := filepath.Join(workRoot, "run")
	workspace := filepath.Join(runDir, "workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	seedCrashProject(t, workspace)
	env := crashBinaryEnv(workspace)
	storePath := filepath.Join(runDir, "memory", "production_work.json")

	first := startCrashBinary(t, bin, runDir, env)
	defer func() {
		if first.cmd.Process != nil {
			killCrashTree(first.cmd.Process.Pid)
		}
	}()

	first.writeLine("Crea un programa Go nuevo llamado sumador_crash con una funcion Add(a, b int) int, una funcion main y un archivo de prueba sumador_crash_test.go con TestAdd que espere 30 segundos y luego verifique Add(2, 3) == 5. Guardalo en el proyecto. Sin dependencias externas.")
	if !first.waitForOutput("autorización explícita", 5*time.Minute) {
		t.Fatalf("no apareció la autorización; salida:\n%s", tailText(first.output(), 60))
	}
	first.writeLine("Sí, autorizo la ejecución pendiente")

	if !waitForGoTestBinary(8 * time.Minute) {
		t.Fatalf("no se observó un proceso go test; salida:\n%s", tailText(first.output(), 60))
	}
	// The requested test sleeps for 30 s. Kill inside that window: the effect
	// files exist, the result is not finalized yet.
	time.Sleep(time.Second)
	if !goTestBinaryRunning() {
		t.Fatalf("go test terminó antes del kill; salida:\n%s", tailText(first.output(), 60))
	}
	killCrashTree(first.cmd.Process.Pid)
	select {
	case <-first.exited:
	case <-time.After(30 * time.Second):
		t.Fatal("el primer proceso no terminó tras el kill")
	}
	waitForNoGoTestBinary(30 * time.Second)

	sessions, err := readCrashWorkStore(storePath)
	if err != nil {
		t.Fatalf("leer WorkStore tras el crash: %v", err)
	}
	workID, crashed, attempt, ok := runningAttemptWithSpec(sessions)
	if !ok {
		t.Fatalf("no quedó un intento running con especificación; store=%+v", sessions)
	}
	if attempt.Result != nil {
		t.Fatalf("el intento ya tenía resultado registrado; no es un crash antes de registrar: %+v", attempt.Result)
	}
	if attempt.Status != "running" {
		t.Fatalf("estado del intento tras el crash = %q, quería running", attempt.Status)
	}
	var plan programming.MultiFilePlan
	if err := json.Unmarshal(attempt.EffectSpecification, &plan); err != nil {
		t.Fatalf("decodificar EffectSpecification: %v", err)
	}
	if len(plan.Changes) == 0 {
		t.Fatal("la especificación persistida no tiene cambios")
	}
	hashes := map[string]string{}
	for _, change := range plan.Changes {
		path := filepath.Join(workspace, filepath.FromSlash(change.File.Path))
		digest, err := sha256File(path)
		if err != nil {
			t.Fatalf("el efecto esperado %s no existe tras el crash: %v", change.File.Path, err)
		}
		hashes[change.File.Path] = digest
	}
	attemptsBefore := len(crashed.Attempts)

	second := startCrashBinary(t, bin, runDir, env)
	defer func() {
		if second.cmd.Process != nil {
			killCrashTree(second.cmd.Process.Pid)
		}
	}()

	var recovered production.WorkSession
	deadline := time.Now().Add(6 * time.Minute)
	for {
		snapshot, err := readCrashWorkStore(storePath)
		if err == nil {
			if w, exists := snapshot[workID]; exists && (w.Status == production.WorkCompleted || w.Status == production.WorkFailed) {
				recovered = w
				break
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("el reinicio no cerró el Work; salida:\n%s", tailText(second.output(), 80))
		}
		time.Sleep(250 * time.Millisecond)
	}
	if !second.waitForOutput("reconciliación", 30*time.Second) {
		t.Fatalf("el binario no anunció la reconciliación; salida:\n%s", tailText(second.output(), 80))
	}

	if recovered.Status != production.WorkCompleted {
		t.Fatalf("Work tras reconciliación = %q, quería completed: %+v", recovered.Status, recovered.LastActionResult)
	}
	if len(recovered.Attempts) != attemptsBefore {
		t.Fatalf("la recuperación repitió el efecto: intentos antes=%d después=%d", attemptsBefore, len(recovered.Attempts))
	}
	recoveredAttempt := recovered.Attempts[attempt.Number-1]
	if recoveredAttempt.Status != "succeeded" {
		t.Fatalf("intento reconciliado = %q, quería succeeded", recoveredAttempt.Status)
	}
	last := recovered.LastActionResult
	if last == nil || !last.Success || !last.Verified || !last.Observed || last.OutcomeStatus != production.OutcomeSucceeded {
		t.Fatalf("resultado reconciliado inválido: %+v", last)
	}
	if last.VerificationSource != "programming.reconciliation" {
		t.Fatalf("el resultado no vino de reconciliación: %+v", last)
	}
	for _, change := range plan.Changes {
		path := filepath.Join(workspace, filepath.FromSlash(change.File.Path))
		digest, err := sha256File(path)
		if err != nil {
			t.Fatalf("archivo %s desapareció tras reconciliar: %v", change.File.Path, err)
		}
		if digest != hashes[change.File.Path] {
			t.Fatalf("la reconciliación reescribió %s; el efecto se repitió", change.File.Path)
		}
	}

	t.Logf("crash real PASS: work=%s intentos=%d (sin repetir), resultado=%s", workID, len(recovered.Attempts), last.VerificationProfile)
}

func seedCrashProject(t *testing.T, root string) {
	t.Helper()
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatalf("crear workspace: %v", err)
	}
}

func crashBinaryEnv(workspace string) []string {
	dropped := map[string]bool{
		"AHMELIA_OLLAMA_NUM_GPU": true,
		"AHMELIA_OLLAMA_NUM_CTX": true,
	}
	env := make([]string, 0, len(os.Environ())+10)
	for _, entry := range os.Environ() {
		name := entry
		if i := strings.IndexByte(entry, '='); i >= 0 {
			name = entry[:i]
		}
		if dropped[strings.ToUpper(name)] {
			continue
		}
		env = append(env, entry)
	}
	return append(env,
		"AHMELIA_ASYNC_PRODUCTION=1",
		"AHMELIA_DISABLE_VOICE=1",
		"AHMELIA_DISABLE_TTS=1",
		"AHMELIA_SKIP_PROGRAM_INDEX_SCAN=1",
		"AHMELIA_WORKSPACE_ROOT="+workspace,
		"AHMELIA_OLLAMA_HTTP_TIMEOUT_MS=300000",
		"AHMELIA_OLLAMA_KEEP_ALIVE=30m",
	)
}

type crashBinaryProcess struct {
	cmd     *exec.Cmd
	stdin   io.WriteCloser
	mu      sync.Mutex
	out     strings.Builder
	exited  chan struct{}
	waitErr error
}

func startCrashBinary(t *testing.T, bin, dir string, env []string) *crashBinaryProcess {
	t.Helper()
	cmd := exec.Command(bin)
	cmd.Dir = dir
	cmd.Env = env
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatalf("stdin: %v", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("stdout: %v", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		t.Fatalf("stderr: %v", err)
	}
	process := &crashBinaryProcess{cmd: cmd, stdin: stdin, exited: make(chan struct{})}
	if err := cmd.Start(); err != nil {
		t.Fatalf("iniciar binario: %v", err)
	}
	consume := func(reader io.Reader) {
		scanner := bufio.NewScanner(reader)
		scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		for scanner.Scan() {
			process.mu.Lock()
			process.out.WriteString(scanner.Text())
			process.out.WriteByte('\n')
			process.mu.Unlock()
		}
	}
	go consume(stdout)
	go consume(stderr)
	go func() {
		process.waitErr = cmd.Wait()
		close(process.exited)
	}()
	return process
}

func (p *crashBinaryProcess) writeLine(line string) {
	if _, err := io.WriteString(p.stdin, line+"\n"); err != nil {
		panic("escribir stdin: " + err.Error())
	}
}

func (p *crashBinaryProcess) output() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.out.String()
}

func (p *crashBinaryProcess) waitForOutput(substr string, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if strings.Contains(p.output(), substr) {
			return true
		}
		select {
		case <-p.exited:
			return strings.Contains(p.output(), substr)
		case <-time.After(100 * time.Millisecond):
		}
	}
	return false
}

func killCrashTree(pid int) {
	_ = exec.Command("taskkill", "/PID", strconv.Itoa(pid), "/T", "/F").Run()
}

func goTestBinaryRunning() bool {
	out, err := exec.Command("tasklist", "/FO", "CSV", "/NH").Output()
	if err != nil {
		return false
	}
	parent := strings.ToLower(filepath.Base(os.Args[0]))
	for _, line := range strings.Split(strings.ToLower(string(out)), "\n") {
		if !strings.Contains(line, ".test.exe") {
			continue
		}
		if strings.Contains(line, parent) {
			continue
		}
		return true
	}
	return false
}

func waitForGoTestBinary(timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if goTestBinaryRunning() {
			return true
		}
		time.Sleep(200 * time.Millisecond)
	}
	return false
}

func waitForNoGoTestBinary(timeout time.Duration) {
	deadline := time.Now().Add(timeout)
	for goTestBinaryRunning() && time.Now().Before(deadline) {
		time.Sleep(200 * time.Millisecond)
	}
}

func readCrashWorkStore(path string) (map[string]production.WorkSession, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var sessions map[string]production.WorkSession
	if err := json.Unmarshal(data, &sessions); err != nil {
		return nil, err
	}
	return sessions, nil
}

func runningAttemptWithSpec(sessions map[string]production.WorkSession) (string, production.WorkSession, production.WorkAttempt, bool) {
	for id, work := range sessions {
		for i := len(work.Attempts) - 1; i >= 0; i-- {
			attempt := work.Attempts[i]
			if attempt.Status != "running" || len(attempt.EffectSpecification) == 0 {
				continue
			}
			return id, work, attempt, true
		}
	}
	return "", production.WorkSession{}, production.WorkAttempt{}, false
}

func sha256File(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

func tailText(text string, lines int) string {
	all := strings.Split(strings.TrimRight(text, "\n"), "\n")
	if len(all) > lines {
		all = all[len(all)-lines:]
	}
	return strings.Join(all, "\n")
}
