package main

import (
	"bufio"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
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
)

// -------------------------
// Session
// -------------------------
var (
	sessions   = make(map[string]string) // sid -> username
	sessMu     sync.Mutex
	sessionTTL = 24 * time.Hour
)
var webRoot = detectWebRoot()

// -------------------------
// Tipos comunes
// -------------------------
type loginReq struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// ====== PACIENTE (MVP ya existente) ======
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
    Alternatives    []string `json:"alternatives,omitempty"`   // ← nuevo
    Urgency         string   `json:"urgency"`
    Warnings        []string `json:"warnings,omitempty"`
    RulesFired      []string `json:"rules_fired"`
    MatchedSymptoms []string `json:"matched_symptoms,omitempty"`
}

// ====== ADMIN: Snapshot de KB ======
type Snapshot struct {
	Symptoms    []Symptom    `json:"symptoms"`
	Diseases    []Disease    `json:"diseases"`
	Medications []Medication `json:"medications"`
}
type Symptom struct {
	ID    string `json:"id"`              // ej: fiebre
	Label string `json:"label,omitempty"` // opcional
}
type Disease struct {
	ID          string   `json:"id"`          // ej: gripe
	Name        string   `json:"name"`        // ej: "Gripe"
	System      string   `json:"system"`      // respiratorio, digestivo, etc.
	Type        string   `json:"type"`        // viral, crónico, etc.
	Description string   `json:"description"` // texto
	Symptoms    []string `json:"symptoms"`    // ids de sintoma
	ContraMeds  []string `json:"contra_meds"` // meds contraindicados para esta enfermedad
}
type Medication struct {
	ID     string   `json:"id"`              // ej: paracetamol
	Label  string   `json:"label,omitempty"` // opcional
	Treats []string `json:"treats"`          // enfermedades que trata
	Contra []string `json:"contra"`          // condiciones/alergias crónicas contra las que está contraindicado
}

// Snapshot "vacío" para cuando no hay KB todavía
func defaultEmptySnapshot() Snapshot {
	return Snapshot{
		Symptoms:    []Symptom{},
		Diseases:    []Disease{},
		Medications: []Medication{},
	}
}

// Snapshot de ejemplo (mínimo) para bootstrap si no existe el .pl
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

// ========= KB de paciente (mock) para /api/diagnose =========
type kbData struct {
	DiseaseSymptoms map[string][]string
	Treats          map[string][]string
	Contraindicated map[string][]string
	Critical        map[string]bool
}

func stubKB() kbData {
	return kbData{
		DiseaseSymptoms: map[string][]string{
			"gripe":      {"fiebre", "tos", "dolor_garganta"},
			"faringitis": {"dolor_garganta", "fiebre"},
			"neumonia":   {"fiebre", "tos", "disnea", "dolor_pecho"},
			"migraña":    {"cefalea", "nausea"},
		},
		Treats: map[string][]string{
			"paracetamol": {"gripe", "migraña"},
			"amoxicilina": {"faringitis", "neumonia"},
			"ibuprofeno":  {"migraña", "gripe"},
		},
		Contraindicated: map[string][]string{
			"paracetamol": {"alergia_paracetamol"},
			"amoxicilina": {"alergia_penicilina"},
			"ibuprofeno":  {"ulcera_gastrica"},
		},
		Critical: map[string]bool{"disnea": true, "dolor_pecho": true}, // ← corregido
	}
}

// -------------------------
// MAIN + rutas
// -------------------------
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

	// Auth
	mux.HandleFunc("/auth/login", handleLogin)
	mux.HandleFunc("/auth/logout", handleLogout)

	// API paciente
	mux.HandleFunc("/api/diagnose", handleDiagnose)

	// API Admin: snapshot KB
	mux.HandleFunc("/api/admin/snapshot", handleAdminSnapshot)

	// Export/Import PL crudo
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

// ----- páginas -----
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

// ----- auth -----
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

// ----- paciente (lógica Prolog) -----
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
		// si no existe aún el .pl del admin, seguimos con vacío
		kb = []byte{}
	}

	// 2) Crear intérprete e inyectar reglas + hechos estáticos
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

	// 3) Asertar hechos de la sesión EN UNA SOLA Exec
	{
		sevW := map[string]int{"leve": 1, "moderado": 2, "severo": 3}
		var b strings.Builder

		for _, s := range req.Symptoms {
			if !s.Present {
				continue
			}
			wgt := sevW[strings.ToLower(s.Severity)]
			if wgt == 0 {
				wgt = 1
			}
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
	urg := "Observación / automanejo (según evolución)"
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

		// síntomas que hicieron match
		var matched []string
		if q, err := p.Query(fmt.Sprintf(`enf_sintoma(%s,S), presente(S,_).`, safeAtom(enfID))); err == nil {
			for q.Next() {
				var s struct{ S string }
				if err := q.Scan(&s); err == nil {
					matched = append(matched, s.S)
				}
			}
			q.Close()
		}

		// medicamentos seguros (todas las opciones) vía Prolog
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

		// Fallback en Go si Prolog no devolvió ninguno:
		if len(safeMeds) == 0 {
			// 1) candidatos que tratan la enfermedad
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
			// 2) set de condiciones del paciente (alergias + crónicas)
			blocked := map[string]struct{}{}
			for _, a := range req.Allergies {
				blocked[safeAtom(a)] = struct{}{}
			}
			for _, c := range req.Chronics {
				blocked[safeAtom(c)] = struct{}{}
			}
			// 3) filtrar candidatos contra contraindicaciones y enf_contra_medicamento
			for _, cand := range cands {
				bad := false
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
				if !bad {
					if q, err := p.Query(fmt.Sprintf(`enf_contra_medicamento(%s,%s).`, safeAtom(enfID), safeAtom(cand))); err == nil {
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

	// 6) Ordenar por afinidad desc y construir respuesta
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
			Alternatives:    alts,          // resto de opciones seguras
			Urgency:         r2.urg,
			Warnings:        []string{},
			RulesFired:      rf,
			MatchedSymptoms: r2.matched,
		})
	}
	resp.Explanations = "Diagnóstico realizado con Ichiban Prolog: afinidad/3, urgencia/1 y medicamento_seguro/2."

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}


func mapUrgency(u string) string {
	switch u {
	case "inmediata":
		return "Consulta médica inmediata sugerida"
	case "consulta":
		return "Consulta médica recomendada"
	default:
		return "Observación / automanejo (según evolución)"
	}
}
func normList(xs []string) []string {
	out := make([]string, 0, len(xs))
	for _, v := range xs {
		v = strings.TrimSpace(strings.ToLower(v))
		if v != "" {
			out = append(out, v)
		}
	}
	return out
}
func contains(xs []string, v string) bool { for _, e := range xs { if e == v { return true } }; return false }
func suggestDrug(disease string, kb kbData, allergies, chronics []string) (string, []string) {
	var warnings []string
outer:
	for drug, diseases := range kb.Treats {
		if !contains(diseases, disease) {
			continue
		}
		cts := kb.Contraindicated[drug]
		for _, a := range allergies {
			if contains(cts, a) {
				warnings = append(warnings, fmt.Sprintf("Evitar %s por alergia: %s", drug, a))
				continue outer
			}
		}
		for _, c := range chronics {
			if contains(cts, c) {
				warnings = append(warnings, fmt.Sprintf("Evitar %s por condición: %s", drug, c))
				continue outer
			}
		}
		return drug, warnings
	}
	return "", warnings
}

// ----- Admin snapshot API -----
var kbPath = filepath.Join("assets", "kb", "medilogic.pl")

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
			http.Error(w, "bad json", http.StatusBadRequest)
			return
		}
		if err := writePLFromSnapshot(snap); err != nil {
			http.Error(w, "cannot write .pl", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func readKB() ([]byte, error) {
	if b, err := os.ReadFile(kbPath); err == nil {
		return b, nil
	}
	alt := filepath.Join("..", "assets", "kb", "medilogic.pl")
	return os.ReadFile(alt)
}
func writeKB(b []byte) error {
	if err := os.WriteFile(kbPath, b, 0644); err == nil {
		return nil
	}
	alt := filepath.Join("..", "assets", "kb", "medilogic.pl")
	return os.WriteFile(alt, b, 0644)
}

// === NUEVO: readRules con strip BOM y mejor error
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
	b := make([]byte, r.ContentLength)
	_, _ = r.Body.Read(b)
	if len(b) == 0 {
		http.Error(w, "empty body", http.StatusBadRequest)
		return
	}
	if err := writeKB(b); err != nil {
		http.Error(w, "cannot write kb", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

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

// Devuelve los hechos presente(S,P) que REALMENTE asertó el backend en ESTA petición
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

	// --- construir un solo bloque de código Prolog con TODOS los hechos
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

	// --- leer lo que quedó en la BD Prolog
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

// ====== NUEVO: Debug de medicamento (con errores detallados y fallback)
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

	// Cargar reglas + KB con errores visibles
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
		// Devuelve el mensaje exacto del parser
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
			"error":     "kb load error",
			"detail":    err.Error(),
			"preview_kb": string(kb),
		})
		return
	}

	// Asertar TODO en una sola Exec
	sevW := map[string]int{"leve": 1, "moderado": 2, "severo": 3}
	var b strings.Builder
	for _, s := range req.Symptoms {
		if !s.Present {
			continue
		}
		wgt := sevW[strings.ToLower(s.Severity)]
		if wgt == 0 {
			wgt = 1
		}
		fmt.Fprintf(&b, "presente(%s,%d).\n", safeAtom(s.ID), wgt)
	}
	for _, a := range req.Allergies {
		fmt.Fprintf(&b, "alergia(%s).\n", safeAtom(a))
	}
	for _, c := range req.Chronics {
		fmt.Fprintf(&b, "cronica(%s).\n", safeAtom(c))
	}
	if err := p.Exec(b.String()); err != nil {
		http.Error(w, "assert error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// 1) Candidatos
	candQ, err := p.Query(fmt.Sprintf(`trata(M,%s).`, safeAtom(enf)))
	if err != nil {
		http.Error(w, "query trata/2 error: "+err.Error(), http.StatusInternalServerError)
		return
	}
	var candidates []string
	for candQ.Next() {
		var m struct{ M string }
		if err := candQ.Scan(&m); err == nil {
			candidates = append(candidates, m.M)
		}
	}
	candQ.Close()
	if candidates == nil {
		candidates = []string{}
	}

	// 2) Seguros (regla)
	safeQ, err := p.Query(fmt.Sprintf(`medicamento_seguro(%s,M).`, safeAtom(enf)))
	if err != nil {
		http.Error(w, "query medicamento_seguro/2 error: "+err.Error(), http.StatusInternalServerError)
		return
	}
	var safe []string
	for safeQ.Next() {
		var m struct{ M string }
		if err := safeQ.Scan(&m); err == nil {
			safe = append(safe, m.M)
		}
	}
	safeQ.Close()
	if safe == nil {
		safe = []string{}
	}

	// 3) Fallback en Go por si la regla no devuelve nada
	fallbackSafe := []string{}
	whyBlocked := map[string][]string{}
	if len(safe) == 0 && len(candidates) > 0 {
		blocked := map[string]struct{}{}
		for _, a := range req.Allergies {
			blocked[safeAtom(a)] = struct{}{}
		}
		for _, c := range req.Chronics {
			blocked[safeAtom(c)] = struct{}{}
		}

		for _, cand := range candidates {
			isBad := false
			// contraindicado(Med, Cond) ∧ Cond ∈ {alergias, crónicas}
			if q, err := p.Query(fmt.Sprintf(`contraindicado(%s,Cond).`, safeAtom(cand))); err == nil {
				for q.Next() {
					var row struct{ Cond string }
					if err := q.Scan(&row); err == nil {
						if _, ok := blocked[row.Cond]; ok {
							isBad = true
							whyBlocked[cand] = append(whyBlocked[cand], "contra: "+row.Cond)
						}
					}
				}
				q.Close()
			}
			// enf_contra_medicamento(Enf, Med) (si existe)
			if !isBad {
				if q, err := p.Query(fmt.Sprintf(`enf_contra_medicamento(%s,%s).`, safeAtom(enf), safeAtom(cand))); err == nil {
					if q.Next() {
						isBad = true
						whyBlocked[cand] = append(whyBlocked[cand], "enf_contra_medicamento")
					}
					q.Close()
				}
			}
			if !isBad {
				fallbackSafe = append(fallbackSafe, cand)
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"disease":       enf,
		"candidates":    candidates,
		"safe":          safe,
		"fallback_safe": fallbackSafe,
		"why_blocked":   whyBlocked,
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
		// Devuelve el mensaje exacto del parser de Prolog
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

// ----- parse/write PL (formato definido) -----
func loadSnapshotFromPL() (Snapshot, error) {
	b, err := readKB()
	if err != nil { // si no existe el archivo, usa snapshot por defecto
		return defaultSnapshot(), nil
	}
	lines := normalizePL(string(b))
	var snap = defaultEmptySnapshot()

	reSint := regexp.MustCompile(`^sintoma\((\w+)\)\.$`)
	reEnf := regexp.MustCompile(`^enfermedad\((\w+),\s*\"([^\"]*)\",\s*(\w+),\s*(\w+)\)\.$`)
	reEnfS := regexp.MustCompile(`^enf_sintoma\((\w+),\s*(\w+)\)\.$`)
	reMed := regexp.MustCompile(`^medicamento\((\w+)\)\.$`)
	reTrat := regexp.MustCompile(`^trata\((\w+),\s*(\w+)\)\.$`)
	reContra := regexp.MustCompile(`^contraindicado\((\w+),\s*(\w+)\)\.$`)
	reEnfContraMed := regexp.MustCompile(`^enf_contra_medicamento\((\w+),\s*(\w+)\)\.$`)

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
			enf := &Disease{ID: id, Name: name, System: system, Type: typ}
			dmap[id] = enf
			continue
		}
		if m := reEnfS.FindStringSubmatch(ln); m != nil {
			enfID, symID := m[1], m[2]
			if d, ok := dmap[enfID]; ok {
				d.Symptoms = append(d.Symptoms, symID)
			} else {
				tmp := &Disease{ID: enfID, Name: enfID, Symptoms: []string{symID}}
				dmap[enfID] = tmp
			}
			continue
		}
		if m := reMed.FindStringSubmatch(ln); m != nil {
			id := m[1]
			if _, ok := mmap[id]; !ok {
				med := &Medication{ID: id}
				mmap[id] = med
			}
			continue
		}
		if m := reTrat.FindStringSubmatch(ln); m != nil {
			medID, enfID := m[1], m[2]
			if med, ok := mmap[medID]; ok {
				med.Treats = append(med.Treats, enfID)
			} else {
				tmp := &Medication{ID: medID, Treats: []string{enfID}}
				mmap[medID] = tmp
			}
			continue
		}
		if m := reContra.FindStringSubmatch(ln); m != nil {
			medID, cond := m[1], m[2]
			if med, ok := mmap[medID]; ok {
				med.Contra = append(med.Contra, cond)
			} else {
				tmp := &Medication{ID: medID, Contra: []string{cond}}
				mmap[medID] = tmp
			}
			continue
		}
		if m := reEnfContraMed.FindStringSubmatch(ln); m != nil {
			enfID, medID := m[1], m[2]
			if d, ok := dmap[enfID]; ok {
				d.ContraMeds = append(d.ContraMeds, medID)
			} else {
				tmp := &Disease{ID: enfID, Name: enfID, ContraMeds: []string{medID}}
				dmap[enfID] = tmp
			}
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

	// ordenar
	sort.Slice(snap.Symptoms, func(i, j int) bool { return snap.Symptoms[i].ID < snap.Symptoms[j].ID })
	sort.Slice(snap.Diseases, func(i, j int) bool { return snap.Diseases[i].ID < snap.Diseases[j].ID })
	sort.Slice(snap.Medications, func(i, j int) bool { return snap.Medications[i].ID < snap.Medications[j].ID })
	return snap, nil
}

func writePLFromSnapshot(s Snapshot) error {
	var b strings.Builder
	bw := bufio.NewWriter(&b)
	fmt.Fprintln(bw, "% ======= MediLogic KB (auto-generado) =======")
	fmt.Fprintln(bw, "% NO editar a mano; use /admin/kb")
	fmt.Fprintln(bw, "")

	// síntomas
	for _, x := range s.Symptoms {
		fmt.Fprintf(bw, "sintoma(%s).\n", safeAtom(x.ID))
	}
	fmt.Fprintln(bw, "")

	// enfermedades
	for _, d := range s.Diseases {
		name := strings.ReplaceAll(d.Name, `"`, `\"`)
		fmt.Fprintf(bw, "enfermedad(%s, \"%s\", %s, %s).\n",
			safeAtom(d.ID), name, safeAtom(d.System), safeAtom(d.Type))
	}
	for _, d := range s.Diseases {
		for _, sym := range d.Symptoms {
			fmt.Fprintf(bw, "enf_sintoma(%s, %s).\n", safeAtom(d.ID), safeAtom(sym))
		}
		for _, cm := range d.ContraMeds {
			fmt.Fprintf(bw, "enf_contra_medicamento(%s, %s).\n", safeAtom(d.ID), safeAtom(cm))
		}
	}
	fmt.Fprintln(bw, "")

	// medicamentos
	for _, m := range s.Medications {
		fmt.Fprintf(bw, "medicamento(%s).\n", safeAtom(m.ID))
	}
	for _, m := range s.Medications {
		for _, dz := range m.Treats {
			fmt.Fprintf(bw, "trata(%s, %s).\n", safeAtom(m.ID), safeAtom(dz))
		}
		for _, c := range m.Contra {
			fmt.Fprintf(bw, "contraindicado(%s, %s).\n", safeAtom(m.ID), safeAtom(c))
		}
	}

	bw.Flush()
	return writeKB([]byte(b.String()))
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
func safeAtom(s string) string {
	s = strings.TrimSpace(s)
	s = strings.ToLower(s)
	s = strings.ReplaceAll(s, " ", "_")
	s = strings.ReplaceAll(s, "-", "_")
	return s
}

// ----- helpers comunes -----
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
