package main

import (
	"bufio"
	"bytes"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

type Disease struct {
	ID, Name, System, Type, Desc string
	Symptoms, ContraMeds         []string
}

func main() {
	input := flag.String("input", "rpa.txt", "archivo de entrada (TXT)")
	host := flag.String("host", "http://localhost:8080", "host del backend")
	outReport := flag.String("report", "", "guardar reporte (opcional)")
	useRobot := flag.Bool("robot", false, "activar automatización con robotgo (login + navegar a /admin_kb)")
	flag.Parse()

	// 1) Parsear
	recs, err := parse(*input)
	must(err, "leyendo %s", *input)
	if len(recs) == 0 { fail("El archivo %s no contiene enfermedades.", *input) }

	// 2) Generar .pl
	pl := buildPL(recs)

	// 3) Importar
	importURL := strings.TrimRight(*host, "/") + "/api/kb/import"
	if err := postPL(importURL, pl); err != nil {
		fail("Importando KB: %v", err)
	}

	// 4) Reporte
	if strings.TrimSpace(*outReport) != "" {
		rep := buildReport(recs)
		if err := os.WriteFile(*outReport, []byte(rep), 0644); err != nil {
			fail("Escribiendo reporte %s: %v", *outReport, err)
		}
	}

	// 5) Robotgo (login + ir al Gestor)
	if *useRobot {
		runAutomation(strings.TrimRight(*host, "/"), recs)
	}

	fmt.Println("OK: KB importada correctamente.")
}

/* ===== Parser TXT ===== */

func parse(path string) ([]Disease, error) {
	f, err := os.Open(path)
	if err != nil { return nil, err }
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
			if line == "" { flush() }
			continue
		}
		k, v, ok := splitKV(line)
		if !ok { continue }
		switch strings.ToLower(k) {
		case "id":          d.ID = v
		case "nombre":      d.Name = v
		case "sistema":     d.System = v
		case "tipo":        d.Type = v
		case "descripcion": d.Desc = v
		case "sintomas":    d.Symptoms = csv(v)
		case "contrameds":  d.ContraMeds = csv(v)
		}
	}
	if err := sc.Err(); err != nil { return nil, err }
	flush()
	return out, nil
}

func splitKV(s string) (string, string, bool) {
	p := strings.SplitN(s, ":", 2)
	if len(p) != 2 { return "", "", false }
	return strings.TrimSpace(p[0]), strings.TrimSpace(p[1]), true
}

func csv(s string) []string {
	if s == "" { return nil }
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		t := strings.TrimSpace(p)
		if t != "" { out = append(out, t) }
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
		if _, ok := seen[a]; ok { continue }
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
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' { b.WriteRune(r) }
	}
	if b.Len() == 0 { return "x" }
	if c := b.String()[0]; c < 'a' || c > 'z' { return "x_" + b.String() }
	return b.String()
}

/* ===== Generación .pl ===== */

func buildPL(recs []Disease) string {
	syms := map[string]struct{}{}
	meds := map[string]struct{}{}

	var sint, enf, desc, links, contra, m bytes.Buffer

	for _, r := range recs {
		for _, s := range r.Symptoms { syms[s] = struct{}{} }
		for _, c := range r.ContraMeds { meds[c] = struct{}{} }
	}

	for s := range syms { fmt.Fprintf(&sint, "sintoma(%s).\n", s) }

	for _, r := range recs {
		name := strings.ReplaceAll(r.Name, `"`, `\"`)
		fmt.Fprintf(&enf,  "enfermedad(%s, \"%s\", %s, %s).\n", r.ID, name, r.System, r.Type)
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

	for c := range meds { fmt.Fprintf(&m, "medicamento(%s).\n", c) }

	var out bytes.Buffer
	fmt.Fprintln(&out, "% ==== KB generada por RPA ====")
	out.Write(sint.Bytes()); fmt.Fprintln(&out, "")
	out.Write(enf.Bytes());  out.Write(desc.Bytes())
	out.Write(links.Bytes());out.Write(contra.Bytes()); fmt.Fprintln(&out, "")
	out.Write(m.Bytes())
	return out.String()
}

/* ===== POST /api/kb/import ===== */

func postPL(url, content string) error {
	req, _ := http.NewRequest(http.MethodPost, url, bytes.NewBufferString(content))
	req.Header.Set("Content-Type", "text/plain; charset=utf-8")
	client := &http.Client{ Timeout: 10 * time.Second }
	resp, err := client.Do(req)
	if err != nil { return err }
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
	fmt.Fprintf(&b, "MediLogic — RPA Carga de Enfermedades\n")
	fmt.Fprintf(&b, "Fecha: %s\n\n", time.Now().Format(time.RFC1123))
	fmt.Fprintf(&b, "Total enfermedades cargadas: %d\n\n", len(recs))
	for _, d := range recs {
		fmt.Fprintf(&b, "- %s (%s)\n", d.Name, d.ID)
		fmt.Fprintf(&b, "  Sistema: %s | Tipo: %s\n", d.System, d.Type)
		if d.Desc != "" { fmt.Fprintf(&b, "  Desc: %s\n", d.Desc) }
		if len(d.Symptoms) > 0 { fmt.Fprintf(&b, "  Síntomas: %s\n", strings.Join(d.Symptoms, ", ")) }
		if len(d.ContraMeds) > 0 { fmt.Fprintf(&b, "  ContraMeds: %s\n", strings.Join(d.ContraMeds, ", ")) }
		fmt.Fprintln(&b)
	}
	return b.String()
}

/* ===== Utils ===== */

func must(err error, ctx string, args ...any) {
	if err != nil {
		if ctx != "" { fmt.Printf("ERROR "+ctx+": %v\n", append(args, err)...) }
		os.Exit(1)
	}
}
func fail(fmtStr string, args ...any) { fmt.Printf("ERROR: "+fmtStr+"\n", args...); os.Exit(1) }
