//go:build robotgo
package main

import (
	"os/exec"
	"runtime"
	"time"

	"github.com/go-vgo/robotgo"
)

const (
	ADMIN_USER = "admin"
	ADMIN_PASS = "123456"

	// Fallback por coordenadas (si los dejas en -1 usa navegación por TAB)
	USER_X  = -1
	USER_Y  = -1
	PASS_X  = -1
	PASS_Y  = -1
	LOGIN_X = -1
	LOGIN_Y = -1
)

func runAutomation(host string, recs []Disease) {
	urlLogin := host + "/login"
	openBrowser(urlLogin)
	robotgo.MilliSleep(1600)

	// Limpia overlays/autocompletado y asegúrate de que el foco esté en la página
	robotgo.KeyTap("escape")
	robotgo.MilliSleep(150)

	// ---- Usuario ----
	if USER_X >= 0 && USER_Y >= 0 {
		robotgo.Move(USER_X, USER_Y)
		robotgo.Click("left")
	} else {
		// Lleva el foco al primer elemento focusable (debe ser el usuario)
		robotgo.KeyTap("tab")
	}
	robotgo.MilliSleep(200)
	robotgo.TypeStr(ADMIN_USER)

	// ---- Contraseña ----
	if PASS_X >= 0 && PASS_Y >= 0 {
		robotgo.Move(PASS_X, PASS_Y)
		robotgo.Click("left")
	} else {
		robotgo.KeyTap("tab")
	}
	robotgo.MilliSleep(200)
	robotgo.TypeStr(ADMIN_PASS)

	// ---- Enviar ----
	robotgo.MilliSleep(180)
	if LOGIN_X >= 0 && LOGIN_Y >= 0 {
		robotgo.Move(LOGIN_X, LOGIN_Y)
		robotgo.Click("left")
	} else {
		robotgo.KeyTap("enter")
	}

	// Ir al Gestor de Conocimiento
	robotgo.MilliSleep(1400)
	robotgo.KeyTap("l", "ctrl")
	robotgo.TypeStr(host + "/admin/kb")
	robotgo.KeyTap("enter")
}

func openBrowser(url string) {
	switch runtime.GOOS {
	case "windows":
		_ = exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
	case "darwin":
		_ = exec.Command("open", url).Start()
	default:
		_ = exec.Command("xdg-open", url).Start()
	}
	time.Sleep(900 * time.Millisecond)
}
