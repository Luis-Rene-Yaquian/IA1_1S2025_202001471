//go:build windows

package main

import (
	"fmt"
	"syscall"
	"time"
	"unsafe"
)

var (
	user32               = syscall.NewLazyDLL("user32.dll")
	procRegisterHotKey   = user32.NewProc("RegisterHotKey")
	procUnregisterHotKey = user32.NewProc("UnregisterHotKey")
	procGetMessageW      = user32.NewProc("GetMessageW")
	procMessageBeep      = user32.NewProc("MessageBeep")
	procGetCursorPos     = user32.NewProc("GetCursorPos")
	procMouseEvent       = user32.NewProc("mouse_event")
)

const (
	WM_HOTKEY     = 0x0312
	MOD_NOREPEAT  = 0x4000
	VK_F8         = 0x77
	VK_F9         = 0x78
	VK_F10        = 0x79
	MESSAGEBEEPOK = 0xFFFFFFFF

	// mouse_event flags:
	MOUSEEVENTF_LEFTDOWN = 0x0002
	MOUSEEVENTF_LEFTUP   = 0x0004
)

type POINT struct {
	X int32
	Y int32
}

type MSG struct {
	hwnd    uintptr
	message uint32
	wParam  uintptr
	lParam  uintptr
	time    uint32
	pt      POINT
}

func registerHotkey(id int, mod uint32, vk uint32) error {
	r, _, e := procRegisterHotKey.Call(
		0,
		uintptr(id),
		uintptr(mod),
		uintptr(vk),
	)
	if r == 0 {
		return fmt.Errorf("RegisterHotKey id=%d failed: %v", id, e)
	}
	return nil
}
func unregisterHotkey(id int) {
	procUnregisterHotKey.Call(0, uintptr(id))
}

func getCursorPos() (int, int, error) {
	var p POINT
	r, _, e := procGetCursorPos.Call(uintptr(unsafe.Pointer(&p)))
	if r == 0 {
		return 0, 0, e
	}
	return int(p.X), int(p.Y), nil
}

func leftClick() {
	// Usa mouse_event para down+up en la posición actual
	procMouseEvent.Call(MOUSEEVENTF_LEFTDOWN, 0, 0, 0, 0)
	time.Sleep(5 * time.Millisecond)
	procMouseEvent.Call(MOUSEEVENTF_LEFTUP, 0, 0, 0, 0)
}

func beep() { procMessageBeep.Call(MESSAGEBEEPOK) }

func main() {
	fmt.Println("🖱️  Hotkeys globales (funcionan aunque estés en Chrome):")
	fmt.Println("    F8  = capturar X,Y")
	fmt.Println("    F9  = capturar X,Y y hacer CLICK izquierdo")
	fmt.Println("    F10 = salir")
	fmt.Println()

	// Registra hotkeys (sin repetición)
	if err := registerHotkey(1, MOD_NOREPEAT, VK_F8); err != nil { fmt.Println(err); return }
	if err := registerHotkey(2, MOD_NOREPEAT, VK_F9); err != nil { fmt.Println(err); unregisterHotkey(1); return }
	if err := registerHotkey(3, MOD_NOREPEAT, VK_F10); err != nil {
		fmt.Println(err)
		unregisterHotkey(2); unregisterHotkey(1); return
	}
	defer unregisterHotkey(3); defer unregisterHotkey(2); defer unregisterHotkey(1)

	var msg MSG
	for {
		// Espera mensajes (incluye WM_HOTKEY)
		r, _, _ := procGetMessageW.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
		if int32(r) <= 0 {
			break // error o WM_QUIT
		}
		if msg.message == WM_HOTKEY {
			switch msg.wParam {
			case 1: // F8
				x, y, _ := getCursorPos()
				fmt.Printf("[F8]  X=%d Y=%d\n", x, y)
				beep()
			case 2: // F9
				x, y, _ := getCursorPos()
				fmt.Printf("[F9]  X=%d Y=%d (click)\n", x, y)
				leftClick()
				beep()
			case 3: // F10
				fmt.Println("Saliendo...")
				return
			}
		}
	}
}
