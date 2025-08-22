# MediLogic — Cascarón con Login (Go + HTML) (patched)

Incluye autodetección de la carpeta `web/` para que funcione tanto si corres desde `medilogic\` con:
```
go run .\backend
```
como si corres desde `medilogic\backend\` con:
```
go run .
```

Credenciales por defecto:
- Usuario: `admin`
- Password: `123456`

Cámbialas con variables de entorno antes de ejecutar:
```powershell
$env:ADMIN_USER="miusuario"
$env:ADMIN_PASS="mipass"
go run .\backend
```
