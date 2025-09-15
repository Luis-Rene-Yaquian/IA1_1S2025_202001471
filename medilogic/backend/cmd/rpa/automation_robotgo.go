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
   Login + SÍNTOMAS (OK) + ENFERMEDADES (TAB) + MEDICAMENTOS (TAB)
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
	
	// Campo de síntomas asociados (coordenadas DESPUÉS del scroll)
	DZ_SYM_ADD_X = 855  // Promedio de tus nuevas coordenadas post-scroll
	DZ_SYM_ADD_Y = 300
	
	// Campo de medicamentos contraindicados (coordenadas DESPUÉS del scroll)
	DZ_CONTRA_X  = 815  // Promedio de tus nuevas coordenadas post-scroll
	DZ_CONTRA_Y  = 357
	
	DZ_SAVE_X   = 794   // Botón Guardar (promedio actualizado)
	DZ_SAVE_Y   = 397

	// Delays
	DZ_TYPE_DELAY_MS       = 45
	DZ_AFTER_FIELD_WAIT_MS = 150
	DZ_AFTER_SAVE_WAIT_MS  = 300
	DZ_ENTER_WAIT_MS       = 200
)

/* ===== MEDICAMENTOS (COORDENADAS EXACTAS CORREGIDAS) ===== */
const (
	// Coordenadas exactas basadas en tus mediciones
	MED_ID_COORD_X = 840  // Promedio de tus coordenadas X
	MED_ID_COORD_Y = 600  // Promedio de tus coordenadas Y
	
	// Delays específicos para medicamentos
	MED_TYPE_DELAY_MS       = 45
	MED_AFTER_FIELD_WAIT_MS = 150
	MED_AFTER_SAVE_WAIT_MS  = 400
	MED_ENTER_WAIT_MS       = 250
	MED_TAB_WAIT_MS         = 300
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

// Función para hacer scroll hacia abajo con la ruedita del mouse
func scrollDown(clicks int) {
	fmt.Printf("  -> Haciendo scroll hacia abajo (%d clicks)...\n", clicks)
	for i := 0; i < clicks; i++ {
		robotgo.Scroll(0, -3) // Scroll hacia abajo (Y negativo)
		robotgo.MilliSleep(150)
	}
	robotgo.MilliSleep(250) // Pausa extra después del scroll
}

// Función para hacer TAB con debug y espera ajustable
func tabWithDebug(label string, waitMs int) {
	fmt.Printf("  -> TAB a %s...\n", label)
	robotgo.KeyTap("tab")
	sleep(waitMs)
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

	/* 4) SÍNTOMAS – crear/actualizar IDs únicos */
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

	/* 5) ENFERMEDADES – Usando coordenadas + TAB para navegación */
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

		// Navegar con TAB desde descripción hasta síntomas asociados
		fmt.Printf("  -> Navegando a síntomas asociados...\n")
		robotgo.KeyTap("tab")
		sleep(300)

		// Agregar síntomas asociados (ya está enfocado el campo)
		if len(d.Symptoms) > 0 {
			fmt.Printf("  -> Agregando %d síntomas...\n", len(d.Symptoms))
			for _, symptom := range d.Symptoms {
				fmt.Printf("    - Agregando síntoma: %s\n", symptom)
				typeInFocused(symptom, DZ_TYPE_DELAY_MS)
				robotgo.KeyTap("enter")
				sleep(DZ_ENTER_WAIT_MS)
			}
		}

		// Navegar con TAB desde síntomas a medicamentos contraindicados
		fmt.Printf("  -> Navegando a medicamentos contraindicados...\n")
		robotgo.KeyTap("tab")
		sleep(300)

		// Agregar medicamentos contraindicados (ya está enfocado el campo)
		if len(d.ContraMeds) > 0 {
			fmt.Printf("  -> Agregando %d medicamentos contraindicados...\n", len(d.ContraMeds))
			for _, med := range d.ContraMeds {
				fmt.Printf("    - Agregando medicamento: %s\n", med)
				typeInFocused(med, DZ_TYPE_DELAY_MS)
				robotgo.KeyTap("enter")
				sleep(DZ_ENTER_WAIT_MS)
			}
		}

		// Navegar con TAB al botón Guardar y presionar ENTER
		fmt.Printf("  -> Navegando al botón Guardar...\n")
		robotgo.KeyTap("tab")
		sleep(200)
		robotgo.KeyTap("enter")  // Presionar el botón Guardar
		sleep(DZ_AFTER_SAVE_WAIT_MS)
		
		// Verificación visual opcional
		fmt.Printf("  ✓ Enfermedad %s guardada\n", d.ID)
	}
	
	fmt.Println("\n[DZ] OK - Todas las enfermedades procesadas.")

	/* 6) MEDICAMENTOS – FLUJO CORREGIDO CON COORDENADAS EXACTAS */
	fmt.Println("[MED] Creando/actualizando MEDICAMENTOS...")
	
	// Crear set único de medicamentos desde ContraMeds de las enfermedades
	medset := map[string]struct{}{}
	for _, d := range recs {
		for _, med := range d.ContraMeds {
			medset[med] = struct{}{}
		}
	}

	if len(medset) > 0 {
		count := 0
		isFirstMedication := true
		
		for med := range medset {
			count++
			fmt.Printf("\n[MED] Procesando medicamento %d/%d: %s\n", count, len(medset), med)
			
			// RESETEO DE NAVEGACIÓN PARA CADA MEDICAMENTO
			if isFirstMedication {
				// Para el primer medicamento, venir desde el botón Guardar de enfermedades
				fmt.Printf("  -> Primer medicamento: navegando desde Guardar de enfermedades...\n")
				robotgo.KeyTap("tab")  // TAB 1: Eliminar enfermedad
				sleep(200)
				robotgo.KeyTap("tab")  // TAB 2: ID medicamento
				sleep(MED_TAB_WAIT_MS)
				isFirstMedication = false
			} else {
				// Para medicamentos siguientes: CLICK DIRECTO en el campo ID
				fmt.Printf("  -> Medicamento %d: reseteo con click directo en campo ID...\n", count)
				
				// Asegurar scroll hacia abajo para que el campo esté visible
				fmt.Printf("  -> Asegurando scroll hacia medicamentos...\n")
				robotgo.Scroll(0, -3) // Scroll hacia abajo
				sleep(300)
				
				// Click directo en el campo ID de medicamentos
				fmt.Printf("  -> Click directo en campo ID (X=%d, Y=%d)...\n", MED_ID_COORD_X, MED_ID_COORD_Y)
				focusHardWin(MED_ID_COORD_X, MED_ID_COORD_Y)
				sleep(400)
			}
			
			// Limpiar y escribir ID del medicamento
			fmt.Printf("  -> Escribiendo ID medicamento: %s\n", med)
			clearInputLocal()
			robotgo.TypeStrDelay(med, MED_TYPE_DELAY_MS)
			sleep(MED_AFTER_FIELD_WAIT_MS)
			
			// TAB para ir al campo "Etiqueta"
			tabWithDebug("campo Etiqueta", MED_TAB_WAIT_MS)
			
			// Escribir etiqueta (usar mismo ID como etiqueta)
			fmt.Printf("  -> Escribiendo etiqueta: %s\n", med)
			clearInputLocal()
			robotgo.TypeStrDelay(med, MED_TYPE_DELAY_MS)
			sleep(MED_AFTER_FIELD_WAIT_MS)
			
			// TAB para ir al campo "Trata (enfermedades)"
			tabWithDebug("campo 'Trata (enfermedades)'", MED_TAB_WAIT_MS)
			
			// Limpiar el campo "Trata"
			clearInputLocal()
			
			// Buscar y agregar enfermedades que contraindican este medicamento
			fmt.Printf("  -> Buscando enfermedades que contraindican %s...\n", med)
			treatedCount := 0
			for _, d := range recs {
				for _, contraMed := range d.ContraMeds {
					if contraMed == med {
						treatedCount++
						fmt.Printf("    -> Agregando enfermedad: %s\n", d.ID)
						robotgo.TypeStrDelay(d.ID, MED_TYPE_DELAY_MS)
						robotgo.KeyTap("enter")
						sleep(MED_ENTER_WAIT_MS)
						break // Solo una vez por enfermedad
					}
				}
			}
			
			if treatedCount == 0 {
				fmt.Printf("    -> ADVERTENCIA: No se encontraron enfermedades para este medicamento\n")
			} else {
				fmt.Printf("    -> Total enfermedades agregadas: %d\n", treatedCount)
			}
			
			// TAB para ir al campo "Contra (alergias/condiciones)"
			tabWithDebug("campo 'Contra (alergias/condiciones)'", MED_TAB_WAIT_MS)
			
			// Limpiar campo "Contra" y dejarlo vacío
			clearInputLocal()
			fmt.Printf("    -> Campo 'Contra' dejado vacío\n")
			
			// TAB para ir al botón "Guardar"
			tabWithDebug("botón Guardar", 200)
			
			// ENTER para guardar el medicamento
			fmt.Printf("  -> ENTER para guardar medicamento\n")
			robotgo.KeyTap("enter")
			sleep(MED_AFTER_SAVE_WAIT_MS)
			
			fmt.Printf("  ✅ Medicamento %s procesado correctamente\n", med)
			
			// Espera adicional entre medicamentos para estabilización
			if count < len(medset) {
				fmt.Printf("  -> Pausa entre medicamentos...\n")
				sleep(600) // Pausa más larga para asegurar que el sistema se estabilice
			}
		}
		fmt.Printf("\n[MED] ✅ COMPLETADOS %d medicamentos exitosamente.\n", len(medset))
	} else {
		fmt.Println("[MED] No hay medicamentos para procesar.")
	}
	
	fmt.Println("\n[🎉] FLUJO TOTALMENTE COMPLETADO: síntomas, enfermedades y medicamentos.")
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