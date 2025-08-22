package main

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// -------------------------
// Simple session store
// -------------------------
var (
	sessions   = make(map[string]string) // sid -> username
	sessMu     sync.Mutex
	sessionTTL = 24 * time.Hour
)

// Detect web root so it works whether you run from project root or /backend
var webRoot = detectWebRoot()

// -------------------------
// Types for login and diagnose
// -------------------------
type loginReq struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type DiagnoseReq struct {
	Symptoms  []SymptomEntry `json:"symptoms"`  // [{id:"fiebre", severity:"moderado", present:true}]
	Allergies []string       `json:"allergies"` // ["alergia_penicilina"]
	Chronics  []string       `json:"chronics"`  // ["asma"]
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
	Affinity        int      `json:"affinity"` // 0-100
	SuggestedDrug   string   `json:"suggested_drug,omitempty"`
	Urgency         string   `json:"urgency"`
	Warnings        []string `json:"warnings,omitempty"`
	RulesFired      []string `json:"rules_fired"`
	MatchedSymptoms []string `json:"matched_symptoms,omitempty"`
}

// -------------------------
// Minimal in-memory "KB" (to be replaced by Prolog later)
// -------------------------
type kbData struct {
	DiseaseSymptoms map[string][]string // enfermedad -> síntomas requeridos
	Treats          map[string][]string // medicamento -> enfermedades que trata
	Contraindicated map[string][]string // medicamento -> alergias/condiciones contraindicadas
	Critical        map[string]bool     // síntomas críticos para urgencia
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
		Critical: map[string]bool{
			"disnea":      true,
			"dolor_pecho": true,
		},
	}
}

// -------------------------
// HTTP server
// -------------------------
func main() {
	mux := http.NewServeMux()

	// Pages
	mux.HandleFunc("/", handleIndex)
	mux.HandleFunc("/login", handleLoginPage)
	mux.HandleFunc("/paciente", handlePacientePage)
	mux.HandleFunc("/admin", handleAdminPage)

	// Auth
	mux.HandleFunc("/auth/login", handleLogin)
	mux.HandleFunc("/auth/logout", handleLogout)

	// API
	mux.HandleFunc("/api/diagnose", handleDiagnose)

	// KB export/import (placeholders listos)
	mux.HandleFunc("/api/kb/export", handleKBExport)
	mux.HandleFunc("/api/kb/import", handleKBImport)

	// Static assets under /web/assets
	mux.Handle("/assets/", http.StripPrefix("/assets/", http.FileServer(http.Dir(filepath.Join(webRoot, "assets")))))

	server := &http.Server{
		Addr:              ":8080",
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}
	log.Printf("Server listening on http://localhost:8080 (web root: %s)\n", webRoot)
	log.Fatal(server.ListenAndServe())
}

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

func handlePacientePage(w http.ResponseWriter, r *http.Request) {
	serveFile(w, r, "paciente.html")
}

func handleAdminPage(w http.ResponseWriter, r *http.Request) {
	_, ok := currentUser(r)
	if !ok {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}
	serveFile(w, r, "admin.html")
}

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
	cookie := &http.Cookie{
		Name:     "sid",
		Value:    sid,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Expires:  time.Now().Add(sessionTTL),
	}
	http.SetCookie(w, cookie)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"ok":true}`))
}

func handleLogout(w http.ResponseWriter, r *http.Request) {
	c, err := r.Cookie("sid")
	if err == nil {
		sessMu.Lock()
		delete(sessions, c.Value)
		sessMu.Unlock()
		c.MaxAge = -1
		c.Expires = time.Unix(0, 0)
		http.SetCookie(w, c)
	}
	w.WriteHeader(http.StatusNoContent)
}

// -------------------------
// Diagnose handler (mock logic to be replaced by Prolog)
// -------------------------
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

	kb := stubKB()

	// Normalize inputs
	seen := map[string]int{} // symptom -> weight by severity
	severityWeight := map[string]int{"leve": 1, "moderado": 2, "severo": 3}
	var presentList []string
	for _, s := range req.Symptoms {
		if !s.Present {
			continue
		}
		sev := strings.ToLower(s.Severity)
		wgt, ok := severityWeight[sev]
		if !ok {
			wgt = 1
		}
		seen[strings.ToLower(s.ID)] = wgt
		presentList = append(presentList, strings.ToLower(s.ID))
	}
	allergies := normList(req.Allergies)
	chronics := normList(req.Chronics)

	// Evaluate each disease
	type scored struct {
		name     string
		aff      float64
		urgency  string
		warnings []string
		matched  []string
		suggested string
		rules    []string
	}
	var results []scored

	for dis, reqSyms := range kb.DiseaseSymptoms {
		// Affinity: weighted coverage of required symptoms
		var sumReq, sumHit int
		var matched []string
		for _, rs := range reqSyms {
			w := 2 // base weight for required symptom
			sumReq += w
			if val, ok := seen[rs]; ok {
				sumHit += w * val // severity contributes
				matched = append(matched, rs)
			}
		}
		aff := 0.0
		if sumReq > 0 {
			aff = float64(sumHit) / float64(sumReq*3) * 100.0 // max severity=3
		}

		// Urgency
		urg := "observacion"
		for s := range seen {
			if kb.Critical[s] && seen[s] >= 2 {
				urg = "inmediata"
				break
			}
		}
		if urg != "inmediata" {
			if contains(presentList, "dolor_pecho") || contains(presentList, "disnea") {
				urg = "consulta"
			} else if aff >= 66 {
				urg = "consulta"
			}
		}

		// Suggest safe drug
		safeDrug, warn := suggestDrug(dis, kb, allergies, chronics)

		// Rules fired (mock explanation)
		rules := []string{fmt.Sprintf("afinidad_%s", dis)}
		if urg == "inmediata" {
			rules = append(rules, "urgencia_criticos")
		}
		if safeDrug == "" {
			rules = append(rules, "sin_farmacos_seguro")
		}

		results = append(results, scored{
			name: dis, aff: aff, urgency: urg, warnings: warn,
			matched: matched, suggested: safeDrug, rules: rules,
		})
	}

	// Sort by affinity desc
	sort.Slice(results, func(i, j int) bool { return results[i].aff > results[j].aff })

	// Build response
	resp := DiagnoseResp{}
	for _, r2 := range results {
		resp.Diagnoses = append(resp.Diagnoses, Diagnosis{
			Disease:         r2.name,
			Affinity:        int(math.Round(r2.aff)),
			SuggestedDrug:   r2.suggested,
			Urgency:         mapUrgency(r2.urgency),
			Warnings:        r2.warnings,
			RulesFired:      r2.rules,
			MatchedSymptoms: r2.matched,
		})
	}
	resp.Explanations = "Simulado (luego Prolog): afinidad ponderada por severidad, urgencia por síntomas críticos, fármacos filtrados por contraindicaciones."

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

func contains(xs []string, v string) bool {
	for _, e := range xs {
		if e == v {
			return true
		}
	}
	return false
}

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
		return drug, warnings // primer fármaco seguro
	}
	return "", warnings
}

// -------------------------
// KB export/import (archivo .pl)
// -------------------------
var kbPath = filepath.Join("assets", "kb", "medilogic.pl")

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

// -------------------------
// Helpers (web, sessions, utils)
// -------------------------
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

func isAuthenticated(r *http.Request) bool {
	_, ok := currentUser(r)
	return ok
}

func constTimeEq(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

func newSessionID() string {
	buf := make([]byte, 32)
	_, _ = rand.Read(buf)
	return hex.EncodeToString(buf)
}

func getenvDefault(k, def string) string {
	v := os.Getenv(k)
	if v == "" {
		return def
	}
	return v
}

func detectWebRoot() string {
	wd, _ := os.Getwd()
	cands := []string{
		filepath.Join(wd, "web"),
		filepath.Join(wd, "..", "web"),
	}
	for _, c := range cands {
		if st, err := os.Stat(c); err == nil && st.IsDir() {
			return c
		}
	}
	return filepath.Join(wd, "web")
}
