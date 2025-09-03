package main

import (
	"bufio"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	iprolog "github.com/ichiban/prolog"
	strconv "strconv"
)

/* ===========================================================
   Session / WebRoot
   =========================================================== */

var (
	sessions   = make(map[string]string) // sid -> username
	sessMu     sync.Mutex
	sessionTTL = 24 * time.Hour
)
var webRoot = detectWebRoot()

/* ===========================================================
   Tipos comunes
   =========================================================== */

type loginReq struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// ====== PACIENTE ======
type DiagnoseReq struct {
	Symptoms  []SymptomEntry `json:"symptoms"`
	Allergies []string       `json:"allergies"`
	Chronics  []string       `json:"chronics"`
}
type SymptomEntry struct {
	ID       string `json:"id"`
	Severity string `json:"severity"` // leve|moderado|severo
	Present  bool   `json:"present"`
}
type DiagnoseResp struct {
	Diagnoses    []Diagnosis `json:"diagnoses"`
	Explanations string      `json:"explanations"`
}
type Diagnosis struct {
	Disease         string   `json:"disease"`
	Affinity        int      `json:"affinity"`
	SuggestedDrug   string   `json:"suggested_drug,omitempty"`
	Alternatives    []string `json:"alternatives,omitempty"`
	Urgency         string   `json:"urgency"`
	Warnings        []string `json:"warnings,omitempty"`
	RulesFired      []string `json:"rules_fired"`
	MatchedSymptoms []string `json:"matched_symptoms,omitempty"`
}

/* ===========================================================
   ADMIN: Snapshot de KB (fuente de verdad del panel)
   =========================================================== */

type Snapshot struct {
	Symptoms    []Symptom    `json:"symptoms"`
	Diseases    []Disease    `json:"diseases"`
	Medications []Medication `json:"medications"`
}
type Symptom struct {
	ID    string `json:"id"`              // ej: fiebre
	Label string `json:"label,omitempty"` // opcional (solo UI)
}
type Disease struct {
	ID          string   `json:"id"`          // ej: gripe
	Name        string   `json:"name"`        // ej: "Gripe"
	System      string   `json:"system"`      // respiratorio, digestivo, etc.
	Type        string   `json:"type"`        // viral, bacteriano, cronico, ...
	Description string   `json:"description"` // opcional, informe/UI
	Symptoms    []string `json:"symptoms"`    // ids de sintoma
	ContraMeds  []string `json:"contra_meds"` // enf_contra_medicamento(Enf, Med)
}
type Medication struct {
	ID     string   `json:"id"`              // ej: paracetamol
	Label  string   `json:"label,omitempty"` // opcional (solo UI)
	Treats []string `json:"treats"`          // trata(Med, Enf)
	Contra []string `json:"contra"`          // contraindicado(Med, Cond)
}

// Snapshot vacío/ejemplo
func defaultEmptySnapshot() Snapshot {
	return Snapshot{
		Symptoms:    []Symptom{},
		Diseases:    []Disease{},
		Medications: []Medication{},
	}
}
func defaultSnapshot() Snapshot {
	return Snapshot{
		Symptoms: []Symptom{
			{ID: "fiebre"}, {ID: "tos"}, {ID: "dolor_garganta"},
			{ID: "disnea"}, {ID: "dolor_pecho"}, {ID: "cefalea"}, {ID: "nausea"},
		},
		Diseases: []Disease{
			{
				ID:          "gripe",
				Name:        "Gripe",
				System:      "respiratorio",
				Type:        "viral",
				Description: "Infección respiratoria alta.",
				Symptoms:    []string{"fiebre", "tos", "dolor_garganta"},
				ContraMeds:  []string{},
			},
		},
		Medications: []Medication{
			{
				ID:     "paracetamol",
				Label:  "Paracetamol",
				Treats: []string{"gripe"},
				Contra: []string{"alergia_paracetamol"},
			},
		},
	}
}

/* ===========================================================
   MAIN + rutas
   =========================================================== */

func main() {
	mux := http.NewServeMux()

	// Páginas
	mux.HandleFunc("/", handleIndex)
	mux.HandleFunc("/login", handleLoginPage)
	mux.HandleFunc("/paciente", handlePacientePage)
	mux.HandleFunc("/admin", handleAdminPage)
	mux.HandleFunc("/admin/kb", handleAdminKBPage)

	// Debug/aux
	mux.HandleFunc("/api/debug/enf", handleDebugEnf)
	mux.HandleFunc("/api/debug/presentes", handleDebugPres)
	mux.HandleFunc("/api/debug/medseguro", handleDebugMedSeguro)
	mux.HandleFunc("/api/debug/rules", handleDebugRules)
	mux.HandleFunc("/api/debug/afinidad", handleDebugAfinidad)

	// Auth
	mux.HandleFunc("/auth/login", handleLogin)
	mux.HandleFunc("/auth/logout", handleLogout)

	// API paciente
	mux.HandleFunc("/api/diagnose", handleDiagnose)
	mux.HandleFunc("/api/symptoms", handlePublicSymptoms) 
	// API Admin: snapshot KB
	mux.HandleFunc("/api/admin/snapshot", handleAdminSnapshot)

	// Export/Import PL crudo (opcional)
	mux.HandleFunc("/api/kb/export", handleKBExport)
	mux.HandleFunc("/api/kb/import", handleKBImport)

	// Assets estáticos
	mux.Handle("/assets/", http.StripPrefix("/assets/", http.FileServer(http.Dir(filepath.Join(webRoot, "assets")))))

	server := &http.Server{
		Addr:              ":8080",
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}
	log.Printf("Server listening on http://localhost:8080 (web root: %s)\n", webRoot)
	log.Fatal(server.ListenAndServe())
}

/* ===========================================================
   Páginas
   =========================================================== */

func handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	serveFile(w, r, "index.html")
}
func handleLoginPage(w http.ResponseWriter, r *http.Request) {
	if isAuthenticated(r) {
		http.Redirect(w, r, "/admin", http.StatusFound)
		return
	}
	serveFile(w, r, "login.html")
}
func handlePacientePage(w http.ResponseWriter, r *http.Request) { serveFile(w, r, "paciente.html") }
func handleAdminPage(w http.ResponseWriter, r *http.Request) {
	if _, ok := currentUser(r); !ok {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}
	serveFile(w, r, "admin.html")
}
func handleAdminKBPage(w http.ResponseWriter, r *http.Request) {
	if _, ok := currentUser(r); !ok {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}
	serveFile(w, r, "admin_kb.html")
}

/* ===========================================================
   Auth
   =========================================================== */

func handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodOptions {
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req loginReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	adminUser := getenvDefault("ADMIN_USER", "admin")
	adminPass := getenvDefault("ADMIN_PASS", "123456")
	if !constTimeEq(req.Username, adminUser) || !constTimeEq(req.Password, adminPass) {
		http.Error(w, "invalid credentials", http.StatusUnauthorized)
		return
	}
	sid := newSessionID()
	sessMu.Lock()
	sessions[sid] = adminUser
	sessMu.Unlock()
	http.SetCookie(w, &http.Cookie{
		Name:     "sid",
		Value:    sid,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Expires:  time.Now().Add(sessionTTL),
	})
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"ok":true}`))
}
func handleLogout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie("sid"); err == nil {
		sessMu.Lock()
		delete(sessions, c.Value)
		sessMu.Unlock()
		c.MaxAge = -1
		c.Expires = time.Unix(0, 0)
		http.SetCookie(w, c)
	}
	w.WriteHeader(http.StatusNoContent)
}

/* ===========================================================
   PACIENTE (lógica Prolog)
   =========================================================== */
func handleDiagnose(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req DiagnoseReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	// 1) Cargar reglas y KB
	rules, err := readRules()
	if err != nil {
		http.Error(w, "rules.pl not found", http.StatusInternalServerError)
		return
	}
	kb, err := readKB()
	if err != nil {
		kb = []byte{}
	}

	// 2) Crear intérprete e inyectar reglas + KB
	p := iprolog.New(nil, nil)
	if err := p.Exec(string(rules)); err != nil {
		log.Println("prolog rules error:", err)
		http.Error(w, "prolog rules error", http.StatusInternalServerError)
		return
	}
	if len(kb) > 0 {
		if err := p.Exec(string(kb)); err != nil {
			log.Println("prolog kb error:", err)
			http.Error(w, "prolog kb error", http.StatusInternalServerError)
			return
		}
	}

	// 3) Asertar hechos de la sesión (limpia + normaliza severidad)
	{
		var b strings.Builder
		// Limpiar hechos de sesión previos
		b.WriteString("retractall(presente(_, _)).\n")
		b.WriteString("retractall(alergia(_)).\n")
		b.WriteString("retractall(cronica(_)).\n")

		normalize := func(sev string) int {
			sev = strings.ToLower(strings.TrimSpace(sev))
			// acepta "1/2/3"
			if n, err := strconv.Atoi(sev); err == nil && n >= 1 && n <= 3 {
				return n
			}
			// o "leve/moderado/severo"
			switch sev {
			case "severo":
				return 3
			case "moderado":
				return 2
			default:
				return 1 // leve por defecto
			}
		}

		for _, s := range req.Symptoms {
			if !s.Present {
				continue
			}
			wgt := normalize(s.Severity)
			fmt.Fprintf(&b, "presente(%s,%d).\n", safeAtom(s.ID), wgt)
		}
		for _, a := range req.Allergies {
			fmt.Fprintf(&b, "alergia(%s).\n", safeAtom(a))
		}
		for _, c := range req.Chronics {
			fmt.Fprintf(&b, "cronica(%s).\n", safeAtom(c))
		}

		if err := p.Exec(b.String()); err != nil {
			log.Println("assert session facts error:", err)
			http.Error(w, "prolog assert error", http.StatusInternalServerError)
			return
		}
	}

	// 4) Urgencia (global según síntomas presentes)
	urg := "Observación recomendada"
	if q, err := p.Query("urgencia(U)."); err == nil {
		for q.Next() {
			var res struct{ U string }
			if err := q.Scan(&res); err == nil && res.U != "" {
				urg = res.U
				break
			}
		}
		q.Close()
	}

	// 5) Recorrer enfermedades y calcular afinidad + medicamentos
	type diagRow struct {
		id, name, urg string
		aff           int
		matched       []string
		safeMeds      []string
	}
	var rows []diagRow

	diseasesQ, err := p.Query(`enfermedad(Enf, Nombre, _, _).`)
	if err != nil {
		http.Error(w, "query enfermedad/4 failed", http.StatusInternalServerError)
		return
	}
	for diseasesQ.Next() {
		var d struct {
			Enf    string
			Nombre string
		}
		if err := diseasesQ.Scan(&d); err != nil {
			continue
		}
		enfID := d.Enf
		enfName := d.Nombre

		// afinidad(Enf, A, _)
		aff := 0
		if q, err := p.Query(fmt.Sprintf(`afinidad(%s, A, _).`, safeAtom(enfID))); err == nil {
			if q.Next() {
				var a struct{ A int }
				if err := q.Scan(&a); err == nil {
					aff = a.A
				}
			}
			q.Close()
		}

		// síntomas que hicieron match (normalizados)
		var matched []string
		if q, err := p.Query(fmt.Sprintf(`enf_sintoma(%s,S), presentepeso(S,_).`, safeAtom(enfID))); err == nil {
			seen := map[string]struct{}{}
			for q.Next() {
				var s struct{ S string }
				if err := q.Scan(&s); err == nil {
					if _, ok := seen[s.S]; !ok {
						matched = append(matched, s.S)
						seen[s.S] = struct{}{}
					}
				}
			}
			q.Close()
		}

		// medicamentos seguros por regla
		safeMeds := []string{}
		if q, err := p.Query(fmt.Sprintf(`medicamento_seguro(%s, M).`, safeAtom(enfID))); err == nil {
			for q.Next() {
				var m struct{ M string }
				if err := q.Scan(&m); err == nil {
					safeMeds = append(safeMeds, m.M)
				}
			}
			q.Close()
		}

		// Fallback si no hubo resultados (filtra contraindicaciones básicas)
		if len(safeMeds) == 0 {
			// candidatos que tratan la enfermedad
			cands := []string{}
			if q, err := p.Query(fmt.Sprintf(`trata(M,%s).`, safeAtom(enfID))); err == nil {
				for q.Next() {
					var m struct{ M string }
					if err := q.Scan(&m); err == nil {
						cands = append(cands, m.M)
					}
				}
				q.Close()
			}
			// bloqueos por alergias/crónicas del paciente
			blocked := map[string]struct{}{}
			for _, a := range req.Allergies {
				blocked[safeAtom(a)] = struct{}{}
			}
			for _, c := range req.Chronics {
				blocked[safeAtom(c)] = struct{}{}
			}
			for _, cand := range cands {
				bad := false
				// contraindicado(Med, Cond)
				if q, err := p.Query(fmt.Sprintf(`contraindicado(%s,Cond).`, safeAtom(cand))); err == nil {
					for q.Next() {
						var row struct{ Cond string }
						if err := q.Scan(&row); err == nil {
							if _, ok := blocked[row.Cond]; ok {
								bad = true
								break
							}
						}
					}
					q.Close()
				}
				// enf_contra_medicamento(Enf, Med)
				if !bad {
					if q, err := p.Query(fmt.Sprintf(
						`enf_contra_medicamento(%s,%s).`, safeAtom(enfID), safeAtom(cand),
					)); err == nil {
						if q.Next() {
							bad = true
						}
						q.Close()
					}
				}
				if !bad {
					safeMeds = append(safeMeds, cand)
				}
			}
		}

		rows = append(rows, diagRow{
			id: enfID, name: enfName, aff: aff, urg: urg, matched: matched, safeMeds: safeMeds,
		})
	}
	diseasesQ.Close()

	// 6) Orden y respuesta
	sort.Slice(rows, func(i, j int) bool { return rows[i].aff > rows[j].aff })

	resp := DiagnoseResp{}
	for _, r2 := range rows {
		rf := []string{"afinidad/3", "urgencia/1", "medicamento_seguro/2"}
		med := ""
		if len(r2.safeMeds) > 0 {
			med = r2.safeMeds[0]
		}
		var alts []string
		if len(r2.safeMeds) > 1 {
			alts = r2.safeMeds[1:]
		}
		resp.Diagnoses = append(resp.Diagnoses, Diagnosis{
			Disease:         r2.name,
			Affinity:        r2.aff,
			SuggestedDrug:   med,
			Alternatives:    alts,
			Urgency:         r2.urg,
			Warnings:        []string{},
			RulesFired:      rf,
			MatchedSymptoms: r2.matched,
		})
	}
	resp.Explanations = "Diagnóstico realizado con Ichiban Prolog: afinidad/3, urgencia/1 y medicamento_seguro/2."

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

/* ===========================================================
   Debug endpoints
   =========================================================== */

func handleDebugEnf(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	if id == "" {
		http.Error(w, "missing ?id=gripe", http.StatusBadRequest)
		return
	}
	kb, err := readKB()
	if err != nil {
		http.Error(w, "kb not found", http.StatusInternalServerError)
		return
	}
	p := iprolog.New(nil, nil)
	if err := p.Exec(string(kb)); err != nil {
		http.Error(w, "kb load error", http.StatusInternalServerError)
		return
	}
	q, err := p.Query(fmt.Sprintf(`enf_sintoma(%s,S).`, safeAtom(id)))
	if err != nil {
		http.Error(w, "query error", http.StatusInternalServerError)
		return
	}
	var out []string
	for q.Next() {
		var s struct{ S string }
		if err := q.Scan(&s); err == nil {
			out = append(out, s.S)
		}
	}
	q.Close()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"id": id, "symptoms": out})
}

func handleDebugPres(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req DiagnoseReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	p := iprolog.New(nil, nil)

	sevW := map[string]int{"leve": 1, "moderado": 2, "severo": 3}
	var asserts []string
	var b strings.Builder

	for _, s := range req.Symptoms {
		if !s.Present {
			continue
		}
		wgt := sevW[strings.ToLower(s.Severity)]
		if wgt == 0 {
			wgt = 1
		}
		line := fmt.Sprintf("presente(%s,%d).", safeAtom(s.ID), wgt)
		asserts = append(asserts, line)
		fmt.Fprintln(&b, line)
	}
	for _, a := range req.Allergies {
		line := fmt.Sprintf("alergia(%s).", safeAtom(a))
		asserts = append(asserts, line)
		fmt.Fprintln(&b, line)
	}
	for _, c := range req.Chronics {
		line := fmt.Sprintf("cronica(%s).", safeAtom(c))
		asserts = append(asserts, line)
		fmt.Fprintln(&b, line)
	}
	if err := p.Exec(b.String()); err != nil {
		http.Error(w, "assert error", http.StatusInternalServerError)
		return
	}

	q, err := p.Query(`presente(S,P).`)
	if err != nil {
		http.Error(w, "query error", http.StatusInternalServerError)
		return
	}
	type pair struct{ S string; P int }
	var out []pair
	for q.Next() {
		var row struct{ S string; P int }
		if err := q.Scan(&row); err == nil {
			out = append(out, pair{S: row.S, P: row.P})
		}
	}
	q.Close()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"received":  req,
		"asserts":   asserts,
		"presentes": out,
	})
}

func handleDebugMedSeguro(w http.ResponseWriter, r *http.Request) {
	var req DiagnoseReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	enf := r.URL.Query().Get("id")
	if enf == "" {
		enf = "gripe"
	}

	rules, err := readRules()
	if err != nil {
		http.Error(w, "readRules error: "+err.Error(), http.StatusInternalServerError)
		return
	}
	kb, err := readKB()
	if err != nil {
		http.Error(w, "readKB error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	p := iprolog.New(nil, nil)
	if err := p.Exec(string(rules)); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]any{
			"error":         "rules load error",
			"detail":        err.Error(),
			"preview_rules": string(rules),
		})
		return
	}
	if err := p.Exec(string(kb)); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]any{
			"error":      "kb load error",
			"detail":     err.Error(),
			"preview_kb": string(kb),
		})
		return
	}

	sevW := map[string]int{"leve": 1, "moderado": 2, "severo": 3}
	var b strings.Builder
	for _, s := range req.Symptoms {
		if !s.Present { continue }
		wgt := sevW[strings.ToLower(s.Severity)]; if wgt==0 { wgt=1 }
		fmt.Fprintf(&b, "presente(%s,%d).\n", safeAtom(s.ID), wgt)
	}
	for _, a := range req.Allergies { fmt.Fprintf(&b, "alergia(%s).\n", safeAtom(a)) }
	for _, c := range req.Chronics { fmt.Fprintf(&b, "cronica(%s).\n", safeAtom(c)) }
	if err := p.Exec(b.String()); err != nil {
		http.Error(w, "assert error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	candQ, err := p.Query(fmt.Sprintf(`trata(M,%s).`, safeAtom(enf)))
	if err != nil { http.Error(w, "query trata/2 error: "+err.Error(), http.StatusInternalServerError); return }
	var candidates []string
	for candQ.Next() {
		var m struct{ M string }
		if err := candQ.Scan(&m); err == nil { candidates = append(candidates, m.M) }
	}
	candQ.Close()
	if candidates == nil { candidates = []string{} }

	safeQ, err := p.Query(fmt.Sprintf(`medicamento_seguro(%s,M).`, safeAtom(enf)))
	if err != nil { http.Error(w, "query medicamento_seguro/2 error: "+err.Error(), http.StatusInternalServerError); return }
	var safe []string
	for safeQ.Next() {
		var m struct{ M string }
		if err := safeQ.Scan(&m); err == nil { safe = append(safe, m.M) }
	}
	safeQ.Close()
	if safe == nil { safe = []string{} }

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"disease":    enf,
		"candidates": candidates,
		"safe":       safe,
	})
}

func handleDebugRules(w http.ResponseWriter, r *http.Request) {
	rules, err := readRules()
	if err != nil {
		http.Error(w, "readRules error: "+err.Error(), http.StatusInternalServerError)
		return
	}
	p := iprolog.New(nil, nil)
	if err := p.Exec(string(rules)); err != nil {
		http.Error(w, "rules exec error: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"ok":     true,
		"size":   len(rules),
		"notice": "rules.pl loaded and parsed OK",
	})
}




// Depura afinidad: lista Reqs, Matched (S,P), Puntaje, Max, Afinidad
func handleDebugAfinidad(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	enf := r.URL.Query().Get("id")
	if enf == "" {
		http.Error(w, "missing ?id=<enfermedad>", http.StatusBadRequest)
		return
	}

	// Lee body como en /api/diagnose para asertar presentes/alergias/crónicas
	var req DiagnoseReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	// Carga rules + KB
	rules, err := readRules()
	if err != nil {
		http.Error(w, "rules.pl not found", http.StatusInternalServerError)
		return
	}
	kb, err := readKB()
	if err != nil {
		http.Error(w, "kb not found", http.StatusInternalServerError)
		return
	}

	p := iprolog.New(nil, nil)
	if err := p.Exec(string(rules)); err != nil {
		http.Error(w, "rules exec error: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if err := p.Exec(string(kb)); err != nil {
		http.Error(w, "kb exec error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Asertar hechos de sesión (limpiando antes)
	var b strings.Builder
	b.WriteString("retractall(presente(_, _)).\n")
	b.WriteString("retractall(alergia(_)).\n")
	b.WriteString("retractall(cronica(_)).\n")

	normalize := func(sev string) int {
		sev = strings.ToLower(strings.TrimSpace(sev))
		if n, err := strconv.Atoi(sev); err == nil && n >= 1 && n <= 3 {
			return n
		}
		switch sev {
		case "severo":
			return 3
		case "moderado":
			return 2
		default:
			return 1
		}
	}
	for _, s := range req.Symptoms {
		if !s.Present { continue }
		fmt.Fprintf(&b, "presente(%s,%d).\n", safeAtom(s.ID), normalize(s.Severity))
	}
	for _, a := range req.Allergies { fmt.Fprintf(&b, "alergia(%s).\n", safeAtom(a)) }
	for _, c := range req.Chronics { fmt.Fprintf(&b, "cronica(%s).\n", safeAtom(c)) }

	if err := p.Exec(b.String()); err != nil {
		http.Error(w, "assert error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Reqs (síntomas requeridos por la enfermedad)
	reqs := []string{}
	if q, err := p.Query(fmt.Sprintf(`enf_sintoma(%s,S).`, safeAtom(enf))); err == nil {
		seen := map[string]struct{}{}
		for q.Next() {
			var row struct{ S string }
			if err := q.Scan(&row); err == nil {
				if _, ok := seen[row.S]; !ok {
					reqs = append(reqs, row.S)
					seen[row.S] = struct{}{}
				}
			}
		}
		q.Close()
	}

	// Matched normalizados (S,P) que contaron para el puntaje
	type mp struct{ S string; P int }
	matched := []mp{}
	if q, err := p.Query(fmt.Sprintf(`enf_sintoma(%s,S), presentepeso(S,P).`, safeAtom(enf))); err == nil {
		seen := map[string]struct{}{}
		for q.Next() {
			var row struct{ S string; P int }
			if err := q.Scan(&row); err == nil {
				key := row.S
				if _, ok := seen[key]; !ok {
					matched = append(matched, mp{S: row.S, P: row.P})
					seen[key] = struct{}{}
				}
			}
		}
		q.Close()
	}

	// Puntaje y máximo (Prolog)
	puntaje := 0
	if q, err := p.Query(fmt.Sprintf(`puntaje_enf(%s,P,_).`, safeAtom(enf))); err == nil {
		if q.Next() {
			var row struct{ P int }
			_ = q.Scan(&row)
			puntaje = row.P
		}
		q.Close()
	}
	max := 0
	if q, err := p.Query(fmt.Sprintf(`max_puntaje_enf(%s,M).`, safeAtom(enf))); err == nil {
		if q.Next() {
			var row struct{ M int }
			_ = q.Scan(&row)
			max = row.M
		}
		q.Close()
	}

	// Afinidad reportada por Prolog (para confirmar)
	afin := 0
	if q, err := p.Query(fmt.Sprintf(`afinidad(%s,A,_).`, safeAtom(enf))); err == nil {
		if q.Next() {
			var row struct{ A int }
			_ = q.Scan(&row)
			afin = row.A
		}
		q.Close()
	}

	// Respuesta
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"id":       enf,
		"reqs":     reqs,        // todos los síntomas requeridos (denominador)
		"matched":  matched,     // los que contaron con su peso
		"puntaje":  puntaje,     // suma de pesos
		"max":      max,         // 3 * #reqs
		"afinidad": afin,        // round(puntaje*100/max)
	})
}

/* ===========================================================
   Admin snapshot API (validación fuerte + writer atómico)
   =========================================================== */

var (
	kbPath = filepath.Join("assets", "kb", "medilogic.pl")
	kbMu   sync.Mutex
)

func handleAdminSnapshot(w http.ResponseWriter, r *http.Request) {
	if _, ok := currentUser(r); !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	switch r.Method {
	case http.MethodGet:
		snap, err := loadSnapshotFromPL()
		if err != nil {
			http.Error(w, "cannot parse .pl", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(snap)

	case http.MethodPost:
		var snap Snapshot
		if err := json.NewDecoder(r.Body).Decode(&snap); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}
		if err := validateSnapshot(&snap); err != nil {
			http.Error(w, "snapshot validation error: "+err.Error(), http.StatusUnprocessableEntity)
			return
		}
		if err := writePLFromSnapshot(snap); err != nil {
			http.Error(w, "cannot write .pl: "+err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}




func handlePublicSymptoms(w http.ResponseWriter, r *http.Request) {
    snap, err := loadSnapshotFromPL()
    if err != nil {
        http.Error(w, "cannot load kb", http.StatusInternalServerError)
        return
    }
    ids := make([]string, 0, len(snap.Symptoms))
    for _, s := range snap.Symptoms {
        ids = append(ids, s.ID)
    }
    sort.Strings(ids)
    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(map[string]any{"symptoms": ids})
}


/* ===========================================================
   Lectura/Escritura de KB (.pl)
   =========================================================== */

func readKB() ([]byte, error) {
	if b, err := os.ReadFile(kbPath); err == nil {
		return b, nil
	}
	alt := filepath.Join("..", "assets", "kb", "medilogic.pl")
	return os.ReadFile(alt)
}
func writeKBAtomic(b []byte) error {
	kbMu.Lock()
	defer kbMu.Unlock()
	tmp := kbPath + ".tmp"
	if err := os.WriteFile(tmp, b, 0644); err != nil {
		return err
	}
	_ = os.Remove(kbPath) // Windows: Rename no sobreescribe
	return os.Rename(tmp, kbPath)
}

func readRules() ([]byte, error) {
	p1 := filepath.Join("assets", "kb", "rules.pl")
	if b, err := os.ReadFile(p1); err == nil {
		return stripBOM(b), nil
	}
	p2 := filepath.Join("..", "assets", "kb", "rules.pl")
	if b, err := os.ReadFile(p2); err == nil {
		return stripBOM(b), nil
	}
	return nil, fmt.Errorf("rules.pl not found in %s or %s", p1, p2)
}
func stripBOM(b []byte) []byte {
	if len(b) >= 3 && b[0] == 0xEF && b[1] == 0xBB && b[2] == 0xBF {
		return b[3:]
	}
	return b
}

func handleKBExport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	b, err := readKB()
	if err != nil {
		http.Error(w, "cannot read kb", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Write(b)
}
func handleKBImport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	body, err := io.ReadAll(r.Body)
	if err != nil || len(body) == 0 {
		http.Error(w, "empty body", http.StatusBadRequest)
		return
	}
	// Escribe sin validar (endpoint “raw”); el panel usa /api/admin/snapshot
	if err := writeKBAtomic(body); err != nil {
		http.Error(w, "cannot write kb: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

/* ===========================================================
   Parser/Writer PL (formato estable y agrupado)
   =========================================================== */

func loadSnapshotFromPL() (Snapshot, error) {
	b, err := readKB()
	if err != nil { // si no existe, usa bootstrap
		return defaultSnapshot(), nil
	}
	lines := normalizePL(string(b))
	var snap = defaultEmptySnapshot()

	reSint := regexp.MustCompile(`^sintoma\((\w+)\)\.$`)
	reEnf := regexp.MustCompile(`^enfermedad\((\w+),\s*\"([^\"]*)\",\s*(\w+),\s*(\w+)\)\.$`)
	reDesc := regexp.MustCompile(`^descripcion_enf\((\w+),\s*\"([^\"]*)\"\)\.$`)
	reEnfS := regexp.MustCompile(`^enf_sintoma\((\w+),\s*(\w+)\)\.$`)
	reEnfContraMed := regexp.MustCompile(`^enf_contra_medicamento\((\w+),\s*(\w+)\)\.$`)
	reMed := regexp.MustCompile(`^medicamento\((\w+)\)\.$`)
	reTrat := regexp.MustCompile(`^trata\((\w+),\s*(\w+)\)\.$`)
	reContra := regexp.MustCompile(`^contraindicado\((\w+),\s*(\w+)\)\.$`)

	dmap := map[string]*Disease{}
	smap := map[string]*Symptom{}
	mmap := map[string]*Medication{}

	for _, ln := range lines {
		if m := reSint.FindStringSubmatch(ln); m != nil {
			id := m[1]
			if _, ok := smap[id]; !ok {
				s := Symptom{ID: id}
				smap[id] = &s
			}
			continue
		}
		if m := reEnf.FindStringSubmatch(ln); m != nil {
			id, name, system, typ := m[1], m[2], m[3], m[4]
			enf := dmap[id]
			if enf == nil {
				enf = &Disease{ID: id}
				dmap[id] = enf
			}
			enf.Name, enf.System, enf.Type = name, system, typ
			continue
		}
		if m := reDesc.FindStringSubmatch(ln); m != nil {
			id, desc := m[1], m[2]
			enf := dmap[id]
			if enf == nil {
				enf = &Disease{ID: id}
				dmap[id] = enf
			}
			enf.Description = desc
			continue
		}
		if m := reEnfS.FindStringSubmatch(ln); m != nil {
			enfID, symID := m[1], m[2]
			enf := dmap[enfID]
			if enf == nil {
				enf = &Disease{ID: enfID}
				dmap[enfID] = enf
			}
			enf.Symptoms = uniq(append(enf.Symptoms, symID))
			continue
		}
		if m := reEnfContraMed.FindStringSubmatch(ln); m != nil {
			enfID, medID := m[1], m[2]
			enf := dmap[enfID]
			if enf == nil {
				enf = &Disease{ID: enfID}
				dmap[enfID] = enf
			}
			enf.ContraMeds = uniq(append(enf.ContraMeds, medID))
			continue
		}
		if m := reMed.FindStringSubmatch(ln); m != nil {
			id := m[1]
			if _, ok := mmap[id]; !ok {
				mmap[id] = &Medication{ID: id}
			}
			continue
		}
		if m := reTrat.FindStringSubmatch(ln); m != nil {
			medID, enfID := m[1], m[2]
			med := mmap[medID]
			if med == nil {
				med = &Medication{ID: medID}
				mmap[medID] = med
			}
			med.Treats = uniq(append(med.Treats, enfID))
			continue
		}
		if m := reContra.FindStringSubmatch(ln); m != nil {
			medID, cond := m[1], m[2]
			med := mmap[medID]
			if med == nil {
				med = &Medication{ID: medID}
				mmap[medID] = med
			}
			med.Contra = uniq(append(med.Contra, cond))
			continue
		}
	}

	// Volcar mapas a slices
	for _, s := range smap {
		snap.Symptoms = append(snap.Symptoms, *s)
	}
	for _, d := range dmap {
		snap.Diseases = append(snap.Diseases, *d)
	}
	for _, m := range mmap {
		snap.Medications = append(snap.Medications, *m)
	}

	// Orden estable
	sort.Slice(snap.Symptoms, func(i, j int) bool { return snap.Symptoms[i].ID < snap.Symptoms[j].ID })
	sort.Slice(snap.Diseases, func(i, j int) bool { return snap.Diseases[i].ID < snap.Diseases[j].ID })
	sort.Slice(snap.Medications, func(i, j int) bool { return snap.Medications[i].ID < snap.Medications[j].ID })
	return snap, nil
}

func writePLFromSnapshot(s Snapshot) error {
	// 1) Normalización + validación fuerte
	if err := validateSnapshot(&s); err != nil {
		return err
	}

	// 2) Orden estable de impresión
	sort.Slice(s.Symptoms, func(i, j int) bool { return s.Symptoms[i].ID < s.Symptoms[j].ID })
	sort.Slice(s.Diseases, func(i, j int) bool { return s.Diseases[i].ID < s.Diseases[j].ID })
	sort.Slice(s.Medications, func(i, j int) bool { return s.Medications[i].ID < s.Medications[j].ID })

	var b strings.Builder
	bw := bufio.NewWriter(&b)
	fmt.Fprintln(bw, "% ======= MediLogic KB (auto-generado) =======")
	fmt.Fprintln(bw, "% NO editar a mano; use /admin/kb")

	// 1) sintoma/1
	fmt.Fprintln(bw, "")
	for _, x := range s.Symptoms {
		fmt.Fprintf(bw, "sintoma(%s).\n", safeAtom(x.ID))
	}

	// 2) enfermedad/4
	fmt.Fprintln(bw, "")
	for _, d := range s.Diseases {
		name := escQuotes(d.Name)
		fmt.Fprintf(bw, "enfermedad(%s, \"%s\", %s, %s).\n",
			safeAtom(d.ID), name, safeAtom(d.System), safeAtom(d.Type))
	}

	// 3) descripcion_enf/2 (opcional)
	for _, d := range s.Diseases {
		desc := strings.TrimSpace(d.Description)
		if desc == "" {
			continue
		}
		fmt.Fprintf(bw, "descripcion_enf(%s, \"%s\").\n", safeAtom(d.ID), escQuotes(desc))
	}

	// 4) enf_sintoma/2
	for _, d := range s.Diseases {
		for _, sym := range d.Symptoms {
			fmt.Fprintf(bw, "enf_sintoma(%s, %s).\n", safeAtom(d.ID), safeAtom(sym))
		}
	}

	// 5) enf_contra_medicamento/2
	for _, d := range s.Diseases {
		for _, cm := range d.ContraMeds {
			fmt.Fprintf(bw, "enf_contra_medicamento(%s, %s).\n", safeAtom(d.ID), safeAtom(cm))
		}
	}

	// 6) medicamento/1
	fmt.Fprintln(bw, "")
	for _, m := range s.Medications {
		fmt.Fprintf(bw, "medicamento(%s).\n", safeAtom(m.ID))
	}

	// 7) trata/2
	for _, m := range s.Medications {
		for _, dz := range m.Treats {
			fmt.Fprintf(bw, "trata(%s, %s).\n", safeAtom(m.ID), safeAtom(dz))
		}
	}

	// 8) contraindicado/2
	for _, m := range s.Medications {
		for _, c := range m.Contra {
			fmt.Fprintf(bw, "contraindicado(%s, %s).\n", safeAtom(m.ID), safeAtom(c))
		}
	}

	bw.Flush()
	return writeKBAtomic([]byte(b.String()))
}

/* ===========================================================
   Validación, Normalización y Utils
   =========================================================== */

var reSafe = regexp.MustCompile(`[^a-z0-9_]+`)

func safeAtom(s string) string {
	t := strings.TrimSpace(strings.ToLower(s))
	t = strings.ReplaceAll(t, " ", "_")
	t = strings.ReplaceAll(t, "-", "_")
	t = reSafe.ReplaceAllString(t, "")
	if t == "" {
		t = "x"
	}
	if !(t[0] >= 'a' && t[0] <= 'z') {
		t = "x_" + t
	}
	return t
}

func uniq(ss []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(ss))
	for _, s := range ss {
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}

func escQuotes(s string) string {
	return strings.ReplaceAll(s, `"`, `\"`)
}

func validateSnapshot(s *Snapshot) error {
	// normalizar IDs/contenido
	for i := range s.Symptoms {
		s.Symptoms[i].ID = safeAtom(s.Symptoms[i].ID)
	}
	for i := range s.Diseases {
		d := &s.Diseases[i]
		d.ID = safeAtom(d.ID)
		d.System = safeAtom(d.System)
		d.Type = safeAtom(d.Type)
		for j := range d.Symptoms {
			d.Symptoms[j] = safeAtom(d.Symptoms[j])
		}
		for j := range d.ContraMeds {
			d.ContraMeds[j] = safeAtom(d.ContraMeds[j])
		}
		d.Symptoms = uniq(d.Symptoms)
		d.ContraMeds = uniq(d.ContraMeds)
	}
	for i := range s.Medications {
		m := &s.Medications[i]
		m.ID = safeAtom(m.ID)
		for j := range m.Treats {
			m.Treats[j] = safeAtom(m.Treats[j])
		}
		for j := range m.Contra {
			m.Contra[j] = safeAtom(m.Contra[j])
		}
		m.Treats = uniq(m.Treats)
		m.Contra = uniq(m.Contra)
	}

	// índices para validar referencias
	symSet := map[string]struct{}{}
	for _, x := range s.Symptoms {
		if x.ID == "" {
			return fmt.Errorf("síntoma con ID vacío")
		}
		symSet[x.ID] = struct{}{}
	}

	disMap := map[string]*Disease{}
	for i := range s.Diseases {
		d := &s.Diseases[i]
		if d.ID == "" {
			return fmt.Errorf("enfermedad con ID vacío")
		}
		if strings.TrimSpace(d.Name) == "" {
			return fmt.Errorf("enfermedad %s: nombre requerido", d.ID)
		}
		if d.System == "" || d.Type == "" {
			return fmt.Errorf("enfermedad %s: system y type son requeridos", d.ID)
		}
		disMap[d.ID] = d
		for _, sid := range d.Symptoms {
			if _, ok := symSet[sid]; !ok {
				return fmt.Errorf("enfermedad %s: síntoma '%s' no existe", d.ID, sid)
			}
		}
	}

	medMap := map[string]*Medication{}
	for i := range s.Medications {
		m := &s.Medications[i]
		if m.ID == "" {
			return fmt.Errorf("medicamento con ID vacío")
		}
		medMap[m.ID] = m
	}

	for _, m := range s.Medications {
		for _, e := range m.Treats {
			if _, ok := disMap[e]; !ok {
				return fmt.Errorf("trata(%s,%s): enfermedad no existe", m.ID, e)
			}
		}
	}
	for _, d := range s.Diseases {
		for _, cm := range d.ContraMeds {
			if _, ok := medMap[cm]; !ok {
				return fmt.Errorf("enf_contra_medicamento(%s,%s): medicamento no existe", d.ID, cm)
			}
		}
	}

	return nil
}

func normalizePL(s string) []string {
	out := []string{}
	sc := bufio.NewScanner(strings.NewReader(s))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "%") {
			continue
		}
		out = append(out, line)
	}
	return out
}

/* ===========================================================
   Helpers
   =========================================================== */

func serveFile(w http.ResponseWriter, r *http.Request, name string) {
	p := filepath.Join(webRoot, name)
	http.ServeFile(w, r, p)
}
func currentUser(r *http.Request) (string, bool) {
	c, err := r.Cookie("sid")
	if err != nil || c.Value == "" {
		return "", false
	}
	sessMu.Lock()
	defer sessMu.Unlock()
	user, ok := sessions[c.Value]
	return user, ok
}
func isAuthenticated(r *http.Request) bool { _, ok := currentUser(r); return ok }
func constTimeEq(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}
func newSessionID() string { buf := make([]byte, 32); _, _ = rand.Read(buf); return hex.EncodeToString(buf) }
func getenvDefault(k, def string) string { v := os.Getenv(k); if v == "" { return def }; return v }
func detectWebRoot() string {
	wd, _ := os.Getwd()
	cands := []string{filepath.Join(wd, "web"), filepath.Join(wd, "..", "web")}
	for _, c := range cands {
		if st, err := os.Stat(c); err == nil && st.IsDir() {
			return c
		}
	}
	return filepath.Join(wd, "web")
}
