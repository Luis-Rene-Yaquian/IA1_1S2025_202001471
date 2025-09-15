//go:build windows && robotgo
package main

import (
	"fmt"
	"os/exec"
	"runtime"
	"syscall"
	"time"

	"github.com/go-vgo/robotgo"
)

/* ==============================
   SOLO SÍNTOMAS (modo lento) + WinAPI multi-monitor
   ============================== */

// Credenciales login
const (
	ADMIN_USER = "admin"
	ADMIN_PASS = "123456"
)

const ADMIN_KB_PATH = "/admin/kb"

// === SÍNTOMAS (usa EXACTAMENTE tus coords) ===
const (
	SYM_ID_X   = 830
	SYM_ID_Y   = 330
	SYM_SAVE_X = 807
	SYM_SAVE_Y = 402
)

// Cámara lenta (sólo síntomas)
const (
	SYM_LIMIT               = 1    // procesa 1 síntoma para observar
	SYM_PRE_FOCUS_WAIT_MS   = 700  // antes de enfocar ID
	SYM_AFTER_FOCUS_WAIT_MS = 300  // tras asegurar foco
	SYM_TYPE_DELAY_MS       = 160  // ms por carácter
	SYM_AFTER_TYPE_WAIT_MS  = 700  // tras escribir
	SYM_AFTER_SAVE_WAIT_MS  = 1000 // tras guardar
)

/* ===== WinAPI (multi-monitor) ===== */
var (
	user32           = syscall.NewLazyDLL("user32.dll")
	procSetCursorPos = user32.NewProc("SetCursorPos")
	procMouseEvent   = user32.NewProc("mouse_event")
)

const (
	MOUSEEVENTF_LEFTDOWN = 0x0002
	MOUSEEVENTF_LEFTUP   = 0x0004
)

func setCursorPos(x, y int) {
	procSetCursorPos.Call(uintptr(x), uintptr(y))
}

func mouseLeftClick() {
	procMouseEvent.Call(MOUSEEVENTF_LEFTDOWN, 0, 0, 0, 0)
	time.Sleep(5 * time.Millisecond)
	procMouseEvent.Call(MOUSEEVENTF_LEFTUP, 0, 0, 0, 0)
}

func winClick(x, y int) {
	setCursorPos(x, y)
	time.Sleep(90 * time.Millisecond)
	mouseLeftClick()
}

func focusHardWin(x, y int) {
	// varios clics muy cortos dentro del input para fijar caret
	winClick(x, y)
	time.Sleep(120 * time.Millisecond)
	setCursorPos(x+4, y+4)
	time.Sleep(30 * time.Millisecond)
	mouseLeftClick()
	time.Sleep(120 * time.Millisecond)
}

func clearInputLocal() {
	// limpia SOLO el campo con selección local (sin Ctrl+A)
	robotgo.KeyTap("end")
	robotgo.MilliSleep(40)
	robotgo.KeyTap("home", "shift")
	robotgo.MilliSleep(50)
	robotgo.KeyTap("backspace")
	robotgo.MilliSleep(40)
}

// usado en login (foco ya en el input por TAB)
func typeClearInFocusedSlow(s string) {
	clearInputLocal()
	if s != "" {
		robotgo.TypeStrDelay(s, SYM_TYPE_DELAY_MS)
	}
}

/* =================================== */

func runAutomation(host string, recs []Disease) {
	// 1) Login
	openBrowser(host + "/login")
	sleep(900)
	robotgo.KeyTap("escape")
	sleep(100)

	robotgo.KeyTap("tab") // Usuario
	sleep(120)
	typeClearInFocusedSlow(ADMIN_USER)
	robotgo.KeyTap("tab") // Password
	sleep(120)
	typeClearInFocusedSlow(ADMIN_PASS)
	robotgo.KeyTap("enter")
	sleep(1300)

	// 2) Ir a /admin/kb (sin scroll global)
	openURL(host + ADMIN_KB_PATH)
	sleep(1100)

	// 3) Síntomas (1 registro, despacio)
	fmt.Println("[SYM] Demo lenta sólo de SÍNTOMAS (1 ítem).")
	symset := map[string]struct{}{}
	for _, d := range recs {
		for _, s := range d.Symptoms {
			symset[s] = struct{}{}
		}
	}

	i := 0
	for s := range symset {
		if SYM_LIMIT > 0 && i >= SYM_LIMIT {
			break
		}
		i++

		fmt.Printf("[SYM] (%d) Foco en ID (%d,%d)\n", i, SYM_ID_X, SYM_ID_Y)
		sleep(SYM_PRE_FOCUS_WAIT_MS)
		focusHardWin(SYM_ID_X, SYM_ID_Y)
		sleep(SYM_AFTER_FOCUS_WAIT_MS)
		clearInputLocal()
		sleep(120)
		robotgo.TypeStrDelay(s, SYM_TYPE_DELAY_MS)
		sleep(SYM_AFTER_TYPE_WAIT_MS)

		fmt.Printf("[SYM] (%d) Guardar (%d,%d)\n", i, SYM_SAVE_X, SYM_SAVE_Y)
		winClick(SYM_SAVE_X, SYM_SAVE_Y)
		sleep(SYM_AFTER_SAVE_WAIT_MS)
	}

	fmt.Println("[SYM] Listo.")
}

/* ==============================
   Helpers comunes
   ============================== */

func openBrowser(url string) {
	switch runtime.GOOS {
	case "windows":
		_ = exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
	case "darwin":
		_ = exec.Command("open", url).Start()
	default:
		_ = exec.Command("xdg-open", url).Start()
	}
	sleep(700)
}

func openURL(url string) {
	robotgo.KeyTap("l", "ctrl")
	robotgo.TypeStr(url)
	robotgo.KeyTap("enter")
}

func sleep(ms int) { robotgo.MilliSleep(ms) }
