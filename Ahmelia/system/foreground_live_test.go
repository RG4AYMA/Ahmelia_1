package system

import (
	"os"
	"strings"
	"testing"
	"time"
)

// TestForegroundLive es una verificación DE ESCRITORIO (no hermética):
// compara el camino rápido (syscalls) contra el respaldo PowerShell en la
// máquina real, con latencias. No corre en suites; se pide a propósito:
//
//	AHMELIA_LIVE_FOREGROUND=1 go test ./system -run TestForegroundLive -v -count=1
func TestForegroundLive(t *testing.T) {
	if os.Getenv("AHMELIA_LIVE_FOREGROUND") != "1" {
		t.Skip("live: requiere escritorio interactivo; set AHMELIA_LIVE_FOREGROUND=1")
	}

	start := time.Now()
	fastName, fastPID, fastErr := getForegroundProcessFast()
	fastDur := time.Since(start)

	start = time.Now()
	psName, psPID, psErr := getForegroundProcessPowerShell()
	psDur := time.Since(start)

	t.Logf("rápido (syscalls):    name=%q pid=%d err=%v costo=%s", fastName, fastPID, fastErr, fastDur)
	t.Logf("respaldo (PowerShell): name=%q pid=%d err=%v costo=%s", psName, psPID, psErr, psDur)

	if fastErr != nil {
		t.Fatalf("camino rápido falló: %v", fastErr)
	}
	if psErr == nil {
		if !strings.EqualFold(fastName, psName) || fastPID != psPID {
			t.Fatalf("divergen: rápido=%q/%d vs powershell=%q/%d", fastName, fastPID, psName, psPID)
		}
	} else {
		t.Logf("PowerShell no pudo comparar (%v); el camino rápido sí resolvió", psErr)
	}
}
