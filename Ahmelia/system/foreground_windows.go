//go:build windows

package system

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"
)

// getForegroundProcessFast resuelve el proceso en foco con syscalls de
// Win32 puros (stdlib): user32.GetForegroundWindow →
// GetWindowThreadProcessId → kernel32.OpenProcess(PROCESS_QUERY_LIMITED_
// INFORMATION) → QueryFullProcessImageNameW. Sin cgo, sin spawn de
// procesos, sin tocar go.mod — costo de microsegundos, apto para 1 Hz.
var (
	modUser32   = syscall.NewLazyDLL("user32.dll")
	modKernel32 = syscall.NewLazyDLL("kernel32.dll")

	procGetForegroundWindow        = modUser32.NewProc("GetForegroundWindow")
	procGetWindowThreadProcessId   = modUser32.NewProc("GetWindowThreadProcessId")
	procOpenProcess                = modKernel32.NewProc("OpenProcess")
	procQueryFullProcessImageNameW = modKernel32.NewProc("QueryFullProcessImageNameW")
	procCloseHandle                = modKernel32.NewProc("CloseHandle")
)

// processQueryLimitedInformation alcanza para leer el nombre de la imagen
// del proceso sin permisos plenos (falla en procesos verdaderamente
// protegidos — caso en que el respaldo PowerShell tampoco puede).
const processQueryLimitedInformation = 0x1000

func getForegroundProcessFast() (string, uint32, error) {
	hwnd, _, _ := procGetForegroundWindow.Call()
	if hwnd == 0 {
		return "", 0, errors.New("sin ventana en foco")
	}

	var pid uint32
	procGetWindowThreadProcessId.Call(hwnd, uintptr(unsafe.Pointer(&pid)))
	if pid == 0 {
		return "", 0, errors.New("pid de la ventana en foco desconocido")
	}

	handle, _, openErr := procOpenProcess.Call(processQueryLimitedInformation, 0, uintptr(pid))
	if handle == 0 {
		return "", 0, fmt.Errorf("OpenProcess(%d): %v", pid, openErr)
	}
	defer procCloseHandle.Call(handle)

	buf := make([]uint16, 32768) // soporta cualquier ruta larga de Windows
	size := uint32(len(buf))
	ok, _, queryErr := procQueryFullProcessImageNameW.Call(handle, 0, uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&size)))
	if ok == 0 {
		return "", 0, fmt.Errorf("QueryFullProcessImageName: %v", queryErr)
	}

	full := syscall.UTF16ToString(buf[:size])
	name := strings.TrimSuffix(filepath.Base(full), ".exe")
	if name == "" || name == "." {
		return "", 0, errors.New("nombre de proceso vacío")
	}
	return name, pid, nil
}
