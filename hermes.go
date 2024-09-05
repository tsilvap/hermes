//go:generate npm run build
package main

import (
	"embed"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/alexedwards/scs/v2"
	"github.com/go-chi/chi/v5"
	_ "github.com/mattn/go-sqlite3"
	"github.com/pelletier/go-toml/v2"
)

type Config struct {
	HTTP    HTTPConfig    `toml:"http"`
	Storage StorageConfig `toml:"storage"`
}

type HTTPConfig struct {
	Addr       string `toml:"addr"`
	Schema     string `toml:"schema"`
	DomainName string `toml:"domain_name"`
}

type StorageConfig struct {
	DBPath           string `toml:"db_path"`
	UploadedFilesDir string `toml:"uploaded_files_dir"`
}

var cfg Config

type StderrLogger struct {
	logger *log.Logger
}

func NewStderrLogger() *StderrLogger {
	logger := log.New(os.Stderr, "", log.LstdFlags)
	return &StderrLogger{logger}
}

func (l *StderrLogger) Error(format string, a ...any) {
	l.logger.Printf("ERROR: "+format+"\n", a...)
}

func (l *StderrLogger) Warn(format string, a ...any) {
	l.logger.Printf("WARN: "+format+"\n", a...)
}

func (l *StderrLogger) Info(format string, a ...any) {
	l.logger.Printf("INFO: "+format+"\n", a...)
}

type App struct {
	Logger *StderrLogger
}

//go:embed static templates
var content embed.FS

var sessionManager *scs.SessionManager

func init() {
	sessionManager = scs.New()
	sessionManager.Lifetime = 365 * 24 * time.Hour
	sessionManager.Cookie.Name = "id"

	configPath := os.Getenv("HERMES_CONFIG")
	if configPath == "" {
		configPath = "/etc/hermes/config.toml"
	}
	var err error
	cfg, err = readConfig(configPath)
	if err != nil {
		panic(err)
	}
	if cfg.HTTP.Addr == "" {
		cfg.HTTP.Addr = "127.0.0.1:8080"
	}
	if cfg.Storage.DBPath == "" {
		cfg.Storage.DBPath = "/var/hermes/hermes.db"
	}
	if cfg.Storage.UploadedFilesDir == "" {
		cfg.Storage.UploadedFilesDir = "/var/hermes/uploaded_files/"
	}
}

func main() {
	logger := NewStderrLogger()

	app := App{Logger: logger}

	r := chi.NewRouter()

	r.Use(sessionManager.LoadAndSave)

	r.Handle("/static/*", http.StripPrefix("/static/", http.FileServer(http.FS(content))))
	r.Get("/", app.index)
	r.Route("/login", func(r chi.Router) {
		r.Get("/", app.loginPage)
		r.Post("/", app.loginAction)
	})
	r.Post("/logout", app.logoutAction)
	r.Route("/text", func(r chi.Router) {
		r.With(redirectToLogin).Get("/", app.uploadTextPage)
		r.With(requireLogin).Post("/", app.uploadTextAction)
	})
	r.Route("/files", func(r chi.Router) {
		r.With(redirectToLogin).Get("/", app.uploadFilePage)
		r.With(requireLogin).Post("/", app.uploadFileAction)
	})
	r.Get("/t/{fileID}", app.textPage)
	r.Get("/u/{fileID}", app.filePage)
	r.Get("/dl/{fileID}", app.getRawFile)

	logger.Info("Serving application on http://%s...", cfg.HTTP.Addr)
	log.Fatal(http.ListenAndServe(cfg.HTTP.Addr, r))
}

func readConfig(path string) (Config, error) {
	var cfg Config
	doc, err := os.ReadFile(path)
	if err != nil {
		return cfg, fmt.Errorf("reading config file: %v", err)
	}
	err = toml.Unmarshal(doc, &cfg)
	if err != nil {
		return cfg, fmt.Errorf("parsing config file: %v", err)
	}
	return cfg, nil
}

func redirectToLogin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !loggedIn(r) {
			sendTo(w, "/login")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func requireLogin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !loggedIn(r) {
			unauthorized(w)
			return
		}
		next.ServeHTTP(w, r)
	})
}
