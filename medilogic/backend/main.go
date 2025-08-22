package main

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Simple session store in memory
var (
	sessions   = make(map[string]string) // sid -> username
	sessMu     sync.Mutex
	sessionTTL = 24 * time.Hour
)

// Detect web root so it works whether you run from project root or /backend
var webRoot = detectWebRoot()

type loginReq struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func main() {
	mux := http.NewServeMux()

	// Routes
	mux.HandleFunc("/", handleIndex)
	mux.HandleFunc("/login", handleLoginPage)
	mux.HandleFunc("/paciente", handlePacientePage)
	mux.HandleFunc("/admin", handleAdminPage)

	mux.HandleFunc("/auth/login", handleLogin)
	mux.HandleFunc("/auth/logout", handleLogout)

	// Static assets under /web/assets if you add them later
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
	username, ok := currentUser(r)
	if !ok {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}
	_ = username // personalize if needed
	serveFile(w, r, "admin.html")
}

func handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodOptions { return }
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
	if v == "" { return def }
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
