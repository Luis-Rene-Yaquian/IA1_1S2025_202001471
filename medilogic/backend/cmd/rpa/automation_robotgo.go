//go:build windows && robotgo
package main

import (
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/go-vgo/robotgo"
)

/* ==============================
   Login + SÍNTOMAS (OK) + ENFERMEDADES (Coordenadas específicas para cada campo)
   ============================== */

const (
	ADMIN_USER    = "admin"
	ADMIN_PASS    = "123456"
	ADMIN_KB_PATH = "/admin/kb"
)

/* ===== SÍNTOMAS (ANCLA y flujo confirmado) ===== */
const (
	SYM_ID_X   = 830
	SYM_ID_Y   = 330
	SYM_SAVE_X = 807
	SYM_SAVE_Y = 402

	SYM_TYPE_DELAY_MS      = 120
	SYM_AFTER_SAVE_WAIT_MS = 320
)

/* ===== ENFERMEDADES (Coordenadas específicas para cada campo) ===== */
const (
	// Coordenadas para cada campo específico (basadas en tus mediciones)
	DZ_ID_X     = 900  // Campo ID (promedio de tus coordenadas)
	DZ_ID_Y     = 706
	DZ_NAME_X   = 900  // Campo Nombre (estimado hacia abajo)
	DZ_NAME_Y   = 736
	DZ_SYS_X    = 900  // Campo Sistema
	DZ_SYS_Y    = 766
	DZ_TYPE_X   = 900  // Campo Tipo
	DZ_TYPE_Y   = 796
	DZ_DESC_X   = 900  // Campo Descripción
	DZ_DESC_Y   = 826
	
	// Campo de síntomas asociados (donde hay que presionar enter)
	DZ_SYM_ADD_X = 860  // Promedio de tus coordenadas para síntomas
	DZ_SYM_ADD_Y = 621
	
	// Campo de medicamentos contraindicados (donde hay que presionar enter)
	DZ_CONTRA_X  = 840  // Promedio de tus coordenadas para medicamentos
	DZ_CONTRA_Y  = 680
	
	DZ_SAVE_X   = 795   // Botón Guardar (promedio de tus coordenadas)
	DZ_SAVE_Y   = 716

	// Delays
	DZ_TYPE_DELAY_MS       = 45
	DZ_AFTER_FIELD_WAIT_MS = 150
	DZ_AFTER_SAVE_WAIT_MS  = 300
	DZ_ENTER_WAIT_MS       = 200
)

/* ===== WinAPI clicks (fiables con multi-monitor/DPI) ===== */
var (
	user32           = syscall.NewLazyDLL("user32.dll")
	procSetCursorPos = user32.NewProc("SetCursorPos")
	procMouseEvent   = user32.NewProc("mouse_event")
)

const (
	MOUSEEVENTF_LEFTDOWN = 0x0002
	MOUSEEVENTF_LEFTUP   = 0x0004
)

func setCursorPos(x, y int) { procSetCursorPos.Call(uintptr(x), uintptr(y)) }
func mouseLeftClick() {
	procMouseEvent.Call(MOUSEEVENTF_LEFTDOWN, 0, 0, 0, 0)
	time.Sleep(8 * time.Millisecond)
	procMouseEvent.Call(MOUSEEVENTF_LEFTUP, 0, 0, 0, 0)
}
func winClick(x, y int) {
	setCursorPos(x, y)
	time.Sleep(100 * time.Millisecond)
	mouseLeftClick()
}
func focusHardWin(x, y int) {
	// Triple clic para asegurar selección completa del campo
	winClick(x, y)
	time.Sleep(80 * time.Millisecond)
	winClick(x, y)
	time.Sleep(50 * time.Millisecond)
	winClick(x, y)
	time.Sleep(120 * time.Millisecond)
}

/* ===== Teclado helpers ===== */

func clearInputLocal() {
	// Seleccionar todo el contenido del campo y limpiarlo
	robotgo.KeyTap("a", "ctrl")
	robotgo.MilliSleep(50)
	robotgo.KeyTap("backspace")
	robotgo.MilliSleep(40)
}

func typeInFocused(s string, perCharDelay int) {
	clearInputLocal()
	if s != "" {
		robotgo.TypeStrDelay(s, perCharDelay)
	}
	robotgo.MilliSleep(80) // Pausa después de escribir
}

// Nueva función para llenar un campo específico por coordenadas
func fillField(x, y int, value string, label string) {
	fmt.Printf("  -> Llenando %s: %s\n", label, value)
	focusHardWin(x, y)
	typeInFocused(value, DZ_TYPE_DELAY_MS)
	sleep(DZ_AFTER_FIELD_WAIT_MS)
}

// Nueva función para agregar elementos que requieren ENTER (síntomas y medicamentos)
func addWithEnter(x, y int, value string, label string) {
	fmt.Printf("  -> Agregando %s: %s\n", label, value)
	focusHardWin(x, y)
	typeInFocused(value, DZ_TYPE_DELAY_MS)
	robotgo.KeyTap("enter")
	sleep(DZ_ENTER_WAIT_MS)
}

/* ============================== RUN ============================== */

func runAutomation(host string, recs []Disease) {
	/* 1) Login */
	fmt.Println("[LOGIN] Iniciando sesión...")
	openBrowser(host + "/login")
	sleep(900)
	robotgo.KeyTap("escape")
	sleep(100)
	robotgo.KeyTap("tab"); sleep(120); typeInFocused(ADMIN_USER, 12)
	robotgo.KeyTap("tab"); sleep(120); typeInFocused(ADMIN_PASS, 12)
	robotgo.KeyTap("enter"); sleep(1300)

	/* 2) Gestor */
	fmt.Println("[NAV] Navegando al gestor KB...")
	openURL(host + ADMIN_KB_PATH)
	sleep(1200)

	/* 3) Fijar tope para que SÍNTOMAS siempre calce */
	robotgo.KeyTap("home", "ctrl")
	sleep(300)

	/* 4) SÍNTOMAS — crear/actualizar IDs únicos */
	fmt.Println("[SYM] Creando/actualizando SÍNTOMAS…")
	symset := map[string]struct{}{}
	for _, d := range recs {
		for _, s := range d.Symptoms {
			symset[s] = struct{}{}
		}
	}
	
	count := 0
	for s := range symset {
		count++
		fmt.Printf("  Síntoma %d/%d: %s\n", count, len(symset), s)
		focusHardWin(SYM_ID_X, SYM_ID_Y)
		typeInFocused(s, SYM_TYPE_DELAY_MS)
		winClick(SYM_SAVE_X, SYM_SAVE_Y)
		sleep(SYM_AFTER_SAVE_WAIT_MS)
	}
	fmt.Println("[SYM] OK.")

	/* 5) ENFERMEDADES — Usando coordenadas específicas para cada campo */
	fmt.Println("[DZ] Creando/actualizando ENFERMEDADES...")
	
	for i, d := range recs {
		fmt.Printf("\n[DZ] Procesando enfermedad %d/%d: %s\n", i+1, len(recs), d.Name)
		
		// Scroll al inicio de la sección de enfermedades para consistencia
		robotgo.KeyTap("home", "ctrl")
		sleep(200)
		
		// Llenar campos básicos usando coordenadas específicas
		fillField(DZ_ID_X, DZ_ID_Y, d.ID, "ID")
		fillField(DZ_NAME_X, DZ_NAME_Y, d.Name, "Nombre")
		fillField(DZ_SYS_X, DZ_SYS_Y, d.System, "Sistema")
		fillField(DZ_TYPE_X, DZ_TYPE_Y, d.Type, "Tipo")
		
		// Si hay descripción, llenarla también
		if strings.TrimSpace(d.Desc) != "" {
			fillField(DZ_DESC_X, DZ_DESC_Y, d.Desc, "Descripción")
		}

		// Agregar síntomas asociados (presionando ENTER para cada uno)
		if len(d.Symptoms) > 0 {
			fmt.Printf("  -> Agregando %d síntomas...\n", len(d.Symptoms))
			for _, symptom := range d.Symptoms {
				addWithEnter(DZ_SYM_ADD_X, DZ_SYM_ADD_Y, symptom, "síntoma")
			}
		}

		// Agregar medicamentos contraindicados (presionando ENTER para cada uno)
		if len(d.ContraMeds) > 0 {
			fmt.Printf("  -> Agregando %d medicamentos contraindicados...\n", len(d.ContraMeds))
			for _, med := range d.ContraMeds {
				addWithEnter(DZ_CONTRA_X, DZ_CONTRA_Y, med, "medicamento contraindicado")
			}
		}

		// Guardar
		fmt.Printf("  -> Guardando enfermedad...\n")
		winClick(DZ_SAVE_X, DZ_SAVE_Y)
		sleep(DZ_AFTER_SAVE_WAIT_MS)
		
		// Verificación visual opcional
		fmt.Printf("  ✓ Enfermedad %s guardada\n", d.ID)
	}
	
	fmt.Println("\n[DZ] OK - Todas las enfermedades procesadas.")
	fmt.Println("[OK] Flujo completado: síntomas y enfermedades.")
}

/* ===== util ===== */

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