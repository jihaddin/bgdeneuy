package main

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"sync"
	"time"
)

// =====================
// Data Structures
// =====================

type PageData struct {
	Error    string
	Success  string
	Username string
}

type User struct {
	Nama     string
	Username string
	Email    string
	Password string
}

type Session struct {
	Username  string
	ExpiresAt time.Time
}

// =====================
// In-memory Storage
// =====================

var (
	users    = map[string]User{"admin": {Nama: "Administrator", Username: "admin", Email: "admin@example.com", Password: "admin123"}}
	sessions = map[string]Session{}
	mu       sync.Mutex
)

// =====================
// Session Helpers
// =====================

func generateToken() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func createSession(username string) string {
	token := generateToken()
	mu.Lock()
	sessions[token] = Session{Username: username, ExpiresAt: time.Now().Add(24 * time.Hour)}
	mu.Unlock()
	return token
}

func getSession(r *http.Request) (Session, bool) {
	cookie, err := r.Cookie("session_token")
	if err != nil {
		return Session{}, false
	}
	mu.Lock()
	session, ok := sessions[cookie.Value]
	mu.Unlock()
	if !ok || time.Now().After(session.ExpiresAt) {
		return Session{}, false
	}
	return session, true
}

func deleteSession(r *http.Request) {
	cookie, err := r.Cookie("session_token")
	if err != nil {
		return
	}
	mu.Lock()
	delete(sessions, cookie.Value)
	mu.Unlock()
}

// =====================
// Templates
// =====================

var templates = template.Must(template.ParseGlob("templates/*.html"))

// =====================
// Middleware: cek login
// =====================

func requireLogin(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, ok := getSession(r); !ok {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		next(w, r)
	}
}

// =====================
// Handlers
// =====================

func indexHandler(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	templates.ExecuteTemplate(w, "index.html", nil)
}

func loginHandler(w http.ResponseWriter, r *http.Request) {
	if _, ok := getSession(r); ok {
		http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
		return
	}
	data := PageData{}
	if r.Method == http.MethodPost {
		username := r.FormValue("username")
		password := r.FormValue("password")
		mu.Lock()
		user, exists := users[username]
		mu.Unlock()
		if exists && user.Password == password {
			token := createSession(username)
			http.SetCookie(w, &http.Cookie{
				Name:     "session_token",
				Value:    token,
				Path:     "/",
				HttpOnly: true,
				Expires:  time.Now().Add(24 * time.Hour),
			})
			http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
			return
		}
		data.Error = "Username atau password salah. Coba lagi."
	}
	templates.ExecuteTemplate(w, "login.html", data)
}

func registerHandler(w http.ResponseWriter, r *http.Request) {
	data := PageData{}
	if r.Method == http.MethodPost {
		nama := r.FormValue("nama")
		username := r.FormValue("username")
		email := r.FormValue("email")
		password := r.FormValue("password")
		konfirmasi := r.FormValue("konfirmasi")

		switch {
		case nama == "" || username == "" || email == "" || password == "":
			data.Error = "Semua field harus diisi."
		case len(password) < 6:
			data.Error = "Password minimal 6 karakter."
		case password != konfirmasi:
			data.Error = "Konfirmasi password tidak cocok."
		default:
			mu.Lock()
			_, exists := users[username]
			mu.Unlock()
			if exists {
				data.Error = "Username sudah digunakan, pilih username lain."
			} else {
				mu.Lock()
				users[username] = User{Nama: nama, Username: username, Email: email, Password: password}
				mu.Unlock()
				log.Printf("User baru terdaftar: %s (%s)\n", nama, username)

				// Langsung buat session dan arahkan ke dashboard
				token := createSession(username)
				http.SetCookie(w, &http.Cookie{
					Name:     "session_token",
					Value:    token,
					Path:     "/",
					HttpOnly: true,
					Expires:  time.Now().Add(24 * time.Hour),
				})
				http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
				return
			}
		}
	}
	templates.ExecuteTemplate(w, "register.html", data)
}

func dashboardHandler(w http.ResponseWriter, r *http.Request) {
	session, _ := getSession(r)
	data := PageData{Username: session.Username}
	templates.ExecuteTemplate(w, "dashboard.html", data)
}

func logoutHandler(w http.ResponseWriter, r *http.Request) {
	deleteSession(r)
	http.SetCookie(w, &http.Cookie{
		Name:    "session_token",
		Value:   "",
		Path:    "/",
		Expires: time.Unix(0, 0),
		MaxAge:  -1,
	})
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

// Game handlers
func gameTebakHandler(w http.ResponseWriter, r *http.Request) {
	templates.ExecuteTemplate(w, "game_tebak.html", nil)
}

func gameTictactoeHandler(w http.ResponseWriter, r *http.Request) {
	templates.ExecuteTemplate(w, "game_tictactoe.html", nil)
}

func gameSnakeHandler(w http.ResponseWriter, r *http.Request) {
	templates.ExecuteTemplate(w, "game_snake.html", nil)
}

func gameKuisHandler(w http.ResponseWriter, r *http.Request) {
	templates.ExecuteTemplate(w, "game_kuis.html", nil)
}

// =====================
// Main
// =====================

func main() {
	fs := http.FileServer(http.Dir("static"))
	http.Handle("/static/", http.StripPrefix("/static/", fs))

	// Public routes
	http.HandleFunc("/", indexHandler)
	http.HandleFunc("/login", loginHandler)
	http.HandleFunc("/register", registerHandler)
	http.HandleFunc("/logout", logoutHandler)

	// Protected routes (harus login)
	http.HandleFunc("/dashboard", requireLogin(dashboardHandler))
	http.HandleFunc("/game/tebak-angka", requireLogin(gameTebakHandler))
	http.HandleFunc("/game/tictactoe", requireLogin(gameTictactoeHandler))
	http.HandleFunc("/game/snake", requireLogin(gameSnakeHandler))
	http.HandleFunc("/game/kuis", requireLogin(gameKuisHandler))

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	fmt.Printf("Server berjalan di http://localhost:%s\n", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}
