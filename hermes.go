package main

import (
	"crypto/rand"
	"fmt"
	"html/template"
	"log"
	"math/big"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

type Config struct {
	HTTP    HTTPConfig    `toml:"http"`
	Storage StorageConfig `toml:"storage"`
}

type HTTPConfig struct {
	Schema     string `toml:"schema"`
	DomainName string `toml:"domain_name"`
}

type StorageConfig struct {
	UploadedFilesDir string `toml:"uploaded_files_dir"`
}

var hermesDir string
var cfg Config

func init() {
	hermesDir := os.Getenv("HERMES_DIR")
	if len(hermesDir) == 0 {
		hermesDir = filepath.Join(os.Getenv("HOME"), ".hermes/")
	}
	var err error
	cfg, err = readConfig(filepath.Join(hermesDir, "config.toml"))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	if len(cfg.Storage.UploadedFilesDir) == 0 {
		cfg.Storage.UploadedFilesDir = filepath.Join(hermesDir, "uploaded/")
	}
}

func main() {
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet || r.Method == http.MethodHead {
			tmpl, err := template.ParseFiles("templates/base.tmpl", "templates/index.tmpl")
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: GET /: parsing template: %v\n", err)
				internalServerError(w)
				return
			}
			err = tmpl.Execute(w, nil)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: GET /: executing template: %v\n", err)
				internalServerError(w)
				return
			}
		} else {
			methodNotAllowed(w, []string{http.MethodGet, http.MethodHead})
		}
	})

	http.HandleFunc("/text", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet || r.Method == http.MethodHead {
			tmpl, err := template.ParseFiles("templates/base.tmpl", "templates/text.tmpl")
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: GET /text: parsing template: %v\n", err)
				internalServerError(w)
				return
			}
			err = tmpl.Execute(w, nil)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: GET /text: executing template: %v\n", err)
				internalServerError(w)
				return
			}
		} else if r.Method == http.MethodPost {
			err := r.ParseForm()
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: POST /text: parsing form: %v", err)
				internalServerError(w)
				return
			}
			if !r.PostForm.Has("input") {
				// TODO: Handle no text input.
				fmt.Printf("%#v\n", r.PostForm)
				internalServerError(w) // Will be changed to BadRequest.
				return
			}
			filename, err := generateTextFileName()
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: POST /text: generating title: %v", err)
				internalServerError(w)
				return
			}
			err = os.WriteFile(filepath.Join(cfg.Storage.UploadedFilesDir, filename), []byte(r.PostForm.Get("input")), 0600)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: POST /text: writing file: %v", err)
				internalServerError(w)
				return
			}

			tmpl, err := template.ParseFiles("templates/base.tmpl", "templates/text-success.tmpl")
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: POST /text: parsing template: %v\n", err)
				internalServerError(w)
				return
			}
			err = tmpl.Execute(w, map[string]string{
				"Link": fmt.Sprintf("%s://%s/u/%s", cfg.HTTP.Schema, cfg.HTTP.DomainName, filename),
			})
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: POST /text: executing template: %v\n", err)
				internalServerError(w)
				return
			}
		} else {
			methodNotAllowed(w, []string{http.MethodGet, http.MethodPost, http.MethodHead})
		}
	})

	http.HandleFunc("/files", func(w http.ResponseWriter, r *http.Request) {
		tmpl, err := template.ParseFiles("templates/base.tmpl", "templates/files.tmpl")
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: GET /files: parsing template: %v\n", err)
			internalServerError(w)
			return
		}
		err = tmpl.Execute(w, nil)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: GET /files: executing template: %v\n", err)
			internalServerError(w)
			return
		}
	})

	http.HandleFunc("/u/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet || r.Method == http.MethodHead {
			tmpl, err := template.ParseFiles("templates/base.tmpl", "templates/u-text.tmpl")
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: GET /u/: parsing template: %v\n", err)
				internalServerError(w)
				return
			}
			filename := strings.TrimPrefix(r.URL.Path, "/u/")
			rawText, err := os.ReadFile(filepath.Join(cfg.Storage.UploadedFilesDir, filename))
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: GET /u/: reading file: %v\n", err)
				internalServerError(w)
				return
			}
			err = tmpl.Execute(w, map[string]string{"Title": filename, "Text": string(rawText)})
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: GET /u/: executing template: %v\n", err)
				internalServerError(w)
				return
			}
		} else {
			methodNotAllowed(w, []string{http.MethodGet, http.MethodHead})
		}
	})

	addr := ":8080"
	fmt.Printf("Serving application on %s...\n", addr)
	log.Fatal(http.ListenAndServe(addr, nil))
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

// internalServerError returns an Internal Server Error response.
func internalServerError(w http.ResponseWriter) {
	w.WriteHeader(http.StatusInternalServerError)
	fmt.Fprintln(w, "Internal Server Error")
}

// methodNotAllowed returns a Method Not Allowed response.
func methodNotAllowed(w http.ResponseWriter, allowedMethods []string) {
	w.Header().Add("Allow", strings.Join(allowedMethods, ", "))
	w.WriteHeader(http.StatusMethodNotAllowed)
	fmt.Fprintln(w, "Method Not Allowed")
}

const letters = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"

// generateTextFileName generates a filename for an uploaded text file.
func generateTextFileName() (string, error) {
	identifier := make([]byte, 8)
	for i := range identifier {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(letters))))
		if err != nil {
			return "", fmt.Errorf("get random integer: %v", err)
		}
		identifier[i] = letters[n.Int64()]
	}

	return fmt.Sprintf("%s.txt", identifier), nil
}
