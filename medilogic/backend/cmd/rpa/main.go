

package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/smtp"
	"os"
	"strings"
	"time"
)

type Disease struct {
	ID, Name, System, Type, Desc string
	Symptoms, ContraMeds         []string
}

// Configuración simple de email
type EmailConfig struct {
	FromEmail   string   `json:"from_email"`
	AppPassword string   `json:"app_password"`
	AdminEmails []string `json:"admin_emails"`
}

func main() {
	input := flag.String("input", "rpa.txt", "archivo de entrada (TXT)")
	host := flag.String("host", "http://localhost:8080", "host del backend")
	outReport := flag.String("report", "", "guardar reporte (opcional)")
	useRobot := flag.Bool("robot", false, "activar automatización con robotgo")
	flag.Parse()

	// Registrar tiempo de inicio para el informe
	startTime := time.Now()

	// 1) Parsear
	recs, err := parse(*input)
	must(err, "leyendo %s", *input)
	if len(recs) == 0 {
		fail("El archivo %s no contiene enfermedades.", *input)
	}

	fmt.Printf("Procesando %d enfermedades desde %s...\n", len(recs), *input)

	// 2) Generar .pl
	pl := buildPL(recs)

	// 3) Importar
	importURL := strings.TrimRight(*host, "/") + "/api/kb/import"
	if err := postPL(importURL, pl); err != nil {
		fail("Importando KB: %v", err)
	}

	// 4) Reporte local
	if strings.TrimSpace(*outReport) != "" {
		rep := buildReport(recs)
		if err := os.WriteFile(*outReport, []byte(rep), 0644); err != nil {
			fail("Escribiendo reporte %s: %v", *outReport, err)
		}
		fmt.Printf("Reporte guardado en: %s\n", *outReport)
	}

	// 5) Robotgo (login + ir al Gestor)
	if *useRobot {
		fmt.Println("Iniciando automatización RPA...")
		runAutomation(strings.TrimRight(*host, "/"), recs)
		fmt.Println("Automatización RPA completada.")

		// 6) ENVÍO AUTOMÁTICO DE EMAIL DESPUÉS DEL RPA
		fmt.Println("\n[EMAIL] Enviando informe de trazabilidad automáticamente...")
		
		// Cargar configuración de email
		emailConfig, err := loadEmailConfig()
		if err != nil {
			fmt.Printf("ADVERTENCIA: %v\n", err)
			fmt.Println("Para recibir informes por email, configura el archivo email_config.json")
			fmt.Println("El RPA se completó exitosamente.")
		} else {
			// Generar informe detallado
			report := generateRPAReport(recs, startTime)
			
			// Guardar informe localmente
			reportFile := fmt.Sprintf("rpa_informe_%s.txt", time.Now().Format("2006-01-02_15-04-05"))
			if err := os.WriteFile(reportFile, []byte(report), 0644); err != nil {
				fmt.Printf("ADVERTENCIA: No se pudo guardar informe: %v\n", reportFile)
			} else {
				fmt.Printf("Informe guardado localmente: %s\n", reportFile)
			}
			
			// Enviar por email
			subject := fmt.Sprintf("MediLogic RPA - Informe Automático (%s)", time.Now().Format("2006-01-02 15:04"))
			if err := sendEmail(emailConfig, subject, report); err != nil {
				fmt.Printf("ADVERTENCIA: Error enviando email: %v\n", err)
				fmt.Printf("El informe está guardado en: %s\n", reportFile)
			} else {
				fmt.Printf(" Informe enviado exitosamente a %d administradores\n", len(emailConfig.AdminEmails))
				for _, email := range emailConfig.AdminEmails {
					fmt.Printf("   - %s\n", email)
				}
			}
		}
	}

	fmt.Println("\n PROCESO COMPLETADO: KB importada y procesada correctamente.")
}

// Cargar configuración de email desde archivo JSON
func loadEmailConfig() (EmailConfig, error) {
	configFile := "email_config.json"
	
	// Si no existe, crear archivo de ejemplo
	if _, err := os.Stat(configFile); os.IsNotExist(err) {
		example := EmailConfig{
			FromEmail:   "tu-email@gmail.com",
			AppPassword: "xxxx xxxx xxxx xxxx", // App Password de Gmail
			AdminEmails: []string{"admin@tuempresa.com", "admin2@tuempresa.com"},
		}
		
		data, _ := json.MarshalIndent(example, "", "  ")
		if err := os.WriteFile(configFile, data, 0644); err != nil {
			return EmailConfig{}, fmt.Errorf("no se pudo crear archivo de configuración: %v", err)
		}
		
		return EmailConfig{}, fmt.Errorf("archivo %s creado. Edítalo con tus credenciales de Gmail", configFile)
	}
	
	// Cargar configuración existente
	data, err := os.ReadFile(configFile)
	if err != nil {
		return EmailConfig{}, fmt.Errorf("error leyendo %s: %v", configFile, err)
	}
	
	var config EmailConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return EmailConfig{}, fmt.Errorf("error parseando %s: %v", configFile, err)
	}
	
	// Validar configuración
	if config.FromEmail == "" || config.AppPassword == "" {
		return EmailConfig{}, fmt.Errorf("from_email y app_password son requeridos en %s", configFile)
	}
	
	if len(config.AdminEmails) == 0 {
		return EmailConfig{}, fmt.Errorf("al menos un admin_email es requerido en %s", configFile)
	}
	
	return config, nil
}

// Generar informe detallado según los requisitos del proyecto
func generateRPAReport(recs []Disease, startTime time.Time) string {
	endTime := time.Now()
	duration := endTime.Sub(startTime)
	
	var report strings.Builder
	
	// Encabezado
	report.WriteString("==============================================\n")
	report.WriteString("     MEDILOGIC - INFORME RPA AUTOMÁTICO\n")
	report.WriteString("==============================================\n\n")
	
	// Información de ejecución
	report.WriteString("INFORMACIÓN DE EJECUCIÓN:\n")
	report.WriteString(fmt.Sprintf("Fecha de inicio: %s\n", startTime.Format("2006-01-02 15:04:05")))
	report.WriteString(fmt.Sprintf("Fecha de fin: %s\n", endTime.Format("2006-01-02 15:04:05")))
	report.WriteString(fmt.Sprintf("Duración total: %v\n", duration.Round(time.Second)))
	report.WriteString("Estado: COMPLETADO EXITOSAMENTE\n\n")
	
	// Estadísticas
	symptoms := make(map[string]bool)
	medications := make(map[string]bool)
	
	for _, d := range recs {
		for _, s := range d.Symptoms {
			symptoms[s] = true
		}
		for _, m := range d.ContraMeds {
			medications[m] = true
		}
	}
	
	report.WriteString("RESUMEN ESTADÍSTICO:\n")
	report.WriteString(fmt.Sprintf("- Enfermedades procesadas: %d\n", len(recs)))
	report.WriteString(fmt.Sprintf("- Síntomas únicos: %d\n", len(symptoms)))
	report.WriteString(fmt.Sprintf("- Medicamentos únicos: %d\n\n", len(medications)))
	
	// Detalle de síntomas procesados
	report.WriteString("SÍNTOMAS PROCESADOS:\n")
	for symptom := range symptoms {
		report.WriteString(fmt.Sprintf("  • %s\n", symptom))
	}
	report.WriteString("\n")
	
	// Detalle de enfermedades procesadas
	report.WriteString("ENFERMEDADES PROCESADAS:\n")
	for i, d := range recs {
		report.WriteString(fmt.Sprintf("%d. %s (ID: %s)\n", i+1, d.Name, d.ID))
		report.WriteString(fmt.Sprintf("   Sistema: %s | Tipo: %s\n", d.System, d.Type))
		if d.Desc != "" {
			report.WriteString(fmt.Sprintf("   Descripción: %s\n", d.Desc))
		}
		if len(d.Symptoms) > 0 {
			report.WriteString(fmt.Sprintf("   Síntomas: %s\n", strings.Join(d.Symptoms, ", ")))
		}
		if len(d.ContraMeds) > 0 {
			report.WriteString(fmt.Sprintf("   ContraMeds: %s\n", strings.Join(d.ContraMeds, ", ")))
		}
		report.WriteString("\n")
	}
	
	// Detalle de medicamentos procesados
	report.WriteString("MEDICAMENTOS PROCESADOS:\n")
	for med := range medications {
		report.WriteString(fmt.Sprintf("  • %s\n", med))
		
		// Mostrar qué enfermedades contraindican este medicamento
		var contraindications []string
		for _, d := range recs {
			for _, contraMed := range d.ContraMeds {
				if contraMed == med {
					contraindications = append(contraindications, d.Name)
					break
				}
			}
		}
		if len(contraindications) > 0 {
			report.WriteString(fmt.Sprintf("    Contraindicado en: %s\n", strings.Join(contraindications, ", ")))
		}
		report.WriteString("\n")
	}
	
	// Trazabilidad del proceso
	report.WriteString("TRAZABILIDAD DEL PROCESO RPA:\n")
	report.WriteString("1.  Lectura de archivo TXT exitosa\n")
	report.WriteString("2.  Creación/actualización de síntomas en KB\n")
	report.WriteString("3.  Procesamiento automático de enfermedades\n")
	report.WriteString("4.  Configuración automática de medicamentos\n")
	report.WriteString("5.  Guardado de cambios en medilogic.pl\n")
	report.WriteString("6.  Envío automático de informe por email\n\n")
	
	// Pie del informe
	report.WriteString("==============================================\n")
	report.WriteString("Este informe garantiza la trazabilidad y\n")
	report.WriteString("consistencia de los cambios realizados por\n")
	report.WriteString("el sistema RPA de MediLogic.\n")
	report.WriteString("\n")
	report.WriteString("Para consultas contactar a los administradores.\n")
	report.WriteString("==============================================\n")
	
	return report.String()
}

// Enviar email usando Gmail SMTP
func sendEmail(config EmailConfig, subject, body string) error {
	// Configuración SMTP para Gmail
	smtpHost := "smtp.gmail.com"
	smtpPort := "587"
	
	// Autenticación
	auth := smtp.PlainAuth("", config.FromEmail, config.AppPassword, smtpHost)
	
	// Construir mensaje
	to := strings.Join(config.AdminEmails, ",")
	message := []byte(fmt.Sprintf(
		"To: %s\r\n"+
			"Subject: %s\r\n"+
			"Content-Type: text/plain; charset=UTF-8\r\n"+
			"\r\n"+
			"%s", to, subject, body))
	
	// Enviar
	return smtp.SendMail(smtpHost+":"+smtpPort, auth, config.FromEmail, config.AdminEmails, message)
}

/* ===== Parser TXT ===== */

func parse(path string) ([]Disease, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var out []Disease
	d := Disease{}
	flush := func() {
		if strings.TrimSpace(d.ID) != "" {
			normalize(&d)
			out = append(out, d)
		}
		d = Disease{}
	}

	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "//") {
			if line == "" {
				flush()
			}
			continue
		}
		k, v, ok := splitKV(line)
		if !ok {
			continue
		}
		switch strings.ToLower(k) {
		case "id":
			d.ID = v
		case "nombre":
			d.Name = v
		case "sistema":
			d.System = v
		case "tipo":
			d.Type = v
		case "descripcion":
			d.Desc = v
		case "sintomas":
			d.Symptoms = csv(v)
		case "contrameds":
			d.ContraMeds = csv(v)
		}
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	flush()
	return out, nil
}

func splitKV(s string) (string, string, bool) {
	p := strings.SplitN(s, ":", 2)
	if len(p) != 2 {
		return "", "", false
	}
	return strings.TrimSpace(p[0]), strings.TrimSpace(p[1]), true
}

func csv(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		t := strings.TrimSpace(p)
		if t != "" {
			out = append(out, t)
		}
	}
	return out
}

func normalize(d *Disease) {
	d.ID = atom(d.ID)
	d.System = atom(d.System)
	d.Type = atom(d.Type)
	d.Symptoms = uniqAtoms(d.Symptoms)
	d.ContraMeds = uniqAtoms(d.ContraMeds)
}

func uniqAtoms(ss []string) []string {
	seen := map[string]struct{}{}
	out := []string{}
	for _, s := range ss {
		a := atom(s)
		if _, ok := seen[a]; ok {
			continue
		}
		seen[a] = struct{}{}
		out = append(out, a)
	}
	return out
}

var repl = strings.NewReplacer(" ", "_", "-", "_")

func atom(s string) string {
	x := strings.ToLower(strings.TrimSpace(s))
	x = repl.Replace(x)
	var b strings.Builder
	for _, r := range x {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' {
			b.WriteRune(r)
		}
	}
	if b.Len() == 0 {
		return "x"
	}
	if c := b.String()[0]; c < 'a' || c > 'z' {
		return "x_" + b.String()
	}
	return b.String()
}

/* ===== Generación .pl ===== */

func buildPL(recs []Disease) string {
	syms := map[string]struct{}{}
	meds := map[string]struct{}{}

	var sint, enf, desc, links, contra, m bytes.Buffer

	for _, r := range recs {
		for _, s := range r.Symptoms {
			syms[s] = struct{}{}
		}
		for _, c := range r.ContraMeds {
			meds[c] = struct{}{}
		}
	}

	for s := range syms {
		fmt.Fprintf(&sint, "sintoma(%s).\n", s)
	}

	for _, r := range recs {
		name := strings.ReplaceAll(r.Name, `"`, `\"`)
		fmt.Fprintf(&enf, "enfermedad(%s, \"%s\", %s, %s).\n", r.ID, name, r.System, r.Type)
		if strings.TrimSpace(r.Desc) != "" {
			d := strings.ReplaceAll(r.Desc, `"`, `\"`)
			fmt.Fprintf(&desc, "descripcion_enf(%s, \"%s\").\n", r.ID, d)
		}
		for _, s := range r.Symptoms {
			fmt.Fprintf(&links, "enf_sintoma(%s, %s).\n", r.ID, s)
		}
		for _, c := range r.ContraMeds {
			fmt.Fprintf(&contra, "enf_contra_medicamento(%s, %s).\n", r.ID, c)
		}
	}

	for c := range meds {
		fmt.Fprintf(&m, "medicamento(%s).\n", c)
	}

	var out bytes.Buffer
	fmt.Fprintln(&out, "% ==== KB generada por RPA ====")
	out.Write(sint.Bytes())
	fmt.Fprintln(&out, "")
	out.Write(enf.Bytes())
	out.Write(desc.Bytes())
	out.Write(links.Bytes())
	out.Write(contra.Bytes())
	fmt.Fprintln(&out, "")
	out.Write(m.Bytes())
	return out.String()
}

/* ===== POST /api/kb/import ===== */

func postPL(url, content string) error {
	req, _ := http.NewRequest(http.MethodPost, url, bytes.NewBufferString(content))
	req.Header.Set("Content-Type", "text/plain; charset=utf-8")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("HTTP %d en import: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return nil
}

/* ===== Reporte ===== */

func buildReport(recs []Disease) string {
	var b strings.Builder
	fmt.Fprintf(&b, "MediLogic – RPA Carga de Enfermedades\n")
	fmt.Fprintf(&b, "Fecha: %s\n\n", time.Now().Format(time.RFC1123))
	fmt.Fprintf(&b, "Total enfermedades cargadas: %d\n\n", len(recs))
	for _, d := range recs {
		fmt.Fprintf(&b, "- %s (%s)\n", d.Name, d.ID)
		fmt.Fprintf(&b, "  Sistema: %s | Tipo: %s\n", d.System, d.Type)
		if d.Desc != "" {
			fmt.Fprintf(&b, "  Desc: %s\n", d.Desc)
		}
		if len(d.Symptoms) > 0 {
			fmt.Fprintf(&b, "  Síntomas: %s\n", strings.Join(d.Symptoms, ", "))
		}
		if len(d.ContraMeds) > 0 {
			fmt.Fprintf(&b, "  ContraMeds: %s\n", strings.Join(d.ContraMeds, ", "))
		}
		fmt.Fprintln(&b)
	}
	return b.String()
}

/* ===== Utils ===== */

func must(err error, ctx string, args ...any) {
	if err != nil {
		if ctx != "" {
			fmt.Printf("ERROR "+ctx+": %v\n", append(args, err)...)
		}
		os.Exit(1)
	}
}

func fail(fmtStr string, args ...any) {
	fmt.Printf("ERROR: "+fmtStr+"\n", args...)
	os.Exit(1)
}