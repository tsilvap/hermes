//go:generate npm run build
package main

import (
	"bytes"
	"crypto/rand"
	"database/sql"
	"embed"
	"encoding/hex"
	"errors"
	"fmt"
	"html/template"
	"io"
	"log"
	"math/big"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/crypto/argon2"

	"github.com/alexedwards/scs/v2"
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

var hermesDir string
var cfg Config

//go:embed static
var static embed.FS

var sessionManager *scs.SessionManager

func init() {
	sessionManager = scs.New()
	sessionManager.Lifetime = 365 * 24 * time.Hour
	sessionManager.Cookie.Name = "id"

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
	if cfg.HTTP.Addr == "" {
		cfg.HTTP.Addr = "127.0.0.1:8080"
	}
	if cfg.Storage.DBPath == "" {
		cfg.Storage.DBPath = filepath.Join(hermesDir, "hermes.db")
	}
	if cfg.Storage.UploadedFilesDir == "" {
		cfg.Storage.UploadedFilesDir = filepath.Join(hermesDir, "uploaded/")
	}

}

func main() {
	mux := http.NewServeMux()

	mux.Handle("/static/", http.FileServer(http.FS(static)))

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet || r.Method == http.MethodHead {
			tmpl, err := template.ParseFiles("templates/base.tmpl", "templates/index.tmpl")
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: GET /: parsing template: %v\n", err)
				internalServerError(w)
				return
			}
			uploadedFiles, err := getUploadedFiles(cfg.Storage.UploadedFilesDir)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: GET /: getting list of uploaded files: %v\n", err)
				internalServerError(w)
				return
			}
			err = tmpl.Execute(w, map[string]any{
				"Authenticated": sessionManager.GetBool(r.Context(), "authenticated"),
				"User":          sessionManager.GetString(r.Context(), "user"),

				"UploadedFiles": uploadedFiles,
			})
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: GET /: executing template: %v\n", err)
				internalServerError(w)
				return
			}
		} else {
			methodNotAllowed(w, []string{http.MethodGet, http.MethodHead})
		}
	})

	mux.HandleFunc("/login", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet || r.Method == http.MethodHead {
			if loggedIn(r) {
				sendTo(w, "/")
				return
			}
			tmpl, err := template.ParseFiles("templates/base.tmpl", "templates/login.tmpl")
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: GET /login: parsing template: %v\n", err)
				internalServerError(w)
				return
			}
			err = tmpl.Execute(w, nil)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: GET /login: executing template: %v\n", err)
				internalServerError(w)
				return
			}
		} else if r.Method == http.MethodPost {
			err := r.ParseForm()
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: POST /login: parsing form: %v", err)
				internalServerError(w)
				return

			}
			if !r.PostForm.Has("username") || !r.PostForm.Has("password") {
				// TODO: Handle missing fields.
				fmt.Printf("%#v\n", r.PostForm)
				internalServerError(w) // Will be changed to BadRequest.
				return
			}
			if err := authenticateUser(r, r.PostForm.Get("username"), r.PostForm.Get("password")); err != nil {
				fmt.Fprintf(os.Stderr, "Error: POST /login: authenticating user %q: %v\n", r.PostForm.Get("username"), err)

				tmpl, err := template.ParseFiles("templates/base.tmpl", "templates/login.tmpl")
				if err != nil {
					fmt.Fprintf(os.Stderr, "Error: POST /login: parsing template: %v\n", err)
					internalServerError(w)
					return
				}
				err = tmpl.Execute(w, map[string]any{"BadLogin": true})
				if err != nil {
					fmt.Fprintf(os.Stderr, "Error: POST /login: executing template: %v\n", err)
					internalServerError(w)
					return
				}
				return
			}
			sendTo(w, "/")
		} else {
			methodNotAllowed(w, []string{http.MethodGet, http.MethodPost, http.MethodHead})
		}
	})

	mux.HandleFunc("/logout", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			if err := sessionManager.Destroy(r.Context()); err != nil {
				fmt.Fprintf(os.Stderr, "Error: POST /logout: clearing session data: %v\n", err)
				internalServerError(w)
				return
			}
			sendTo(w, "/")
		} else {
			methodNotAllowed(w, []string{http.MethodPost})
		}
	})

	mux.HandleFunc("/text", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet || r.Method == http.MethodHead {
			if !loggedIn(r) {
				sendTo(w, "/login")
				return
			}
			tmpl, err := template.ParseFiles("templates/base.tmpl", "templates/text.tmpl")
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: GET /text: parsing template: %v\n", err)
				internalServerError(w)
				return
			}
			err = tmpl.Execute(w, map[string]any{
				"Authenticated": sessionManager.GetBool(r.Context(), "authenticated"),
				"User":          sessionManager.GetString(r.Context(), "user"),
			})
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: GET /text: executing template: %v\n", err)
				internalServerError(w)
				return
			}
		} else if r.Method == http.MethodPost {
			if !loggedIn(r) {
				unauthorized(w)
				return
			}
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
			db, err := sql.Open("sqlite3", cfg.Storage.DBPath)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: POST /text: %v", err)
				internalServerError(w)
				return
			}
			defer db.Close()
			stmt, err := db.Prepare(`insert into uploaded_files(title, uploader, file_path, created_at) values(?, ?, ?, ?)`)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: POST /text: %v", err)
				internalServerError(w)
				return
			}
			defer stmt.Close()
			_, err = stmt.Exec(filename, sessionManager.GetString(r.Context(), "user"), filename, time.Now().Unix())
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: POST /text: %v", err)
				internalServerError(w)
				return
			}

			tmpl, err := template.ParseFiles("templates/base.tmpl", "templates/upload-success.tmpl")
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: POST /text: parsing template: %v\n", err)
				internalServerError(w)
				return
			}
			err = tmpl.Execute(w, map[string]any{
				"Authenticated": sessionManager.GetBool(r.Context(), "authenticated"),
				"User":          sessionManager.GetString(r.Context(), "user"),

				"Link": fmt.Sprintf("%s://%s/t/%s", cfg.HTTP.Schema, cfg.HTTP.DomainName, filename),
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

	mux.HandleFunc("/files", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet || r.Method == http.MethodHead {
			if !loggedIn(r) {
				sendTo(w, "/login")
				return
			}
			tmpl, err := template.ParseFiles("templates/base.tmpl", "templates/files.tmpl")
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: GET /files: parsing template: %v\n", err)
				internalServerError(w)
				return
			}
			err = tmpl.Execute(w, map[string]any{
				"Authenticated": sessionManager.GetBool(r.Context(), "authenticated"),
				"User":          sessionManager.GetString(r.Context(), "user"),
			})
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: GET /files: executing template: %v\n", err)
				internalServerError(w)
				return
			}
		} else if r.Method == http.MethodPost {
			if !loggedIn(r) {
				unauthorized(w)
				return
			}
			err := r.ParseMultipartForm(1 << 20) // 1 MB (max. upload size)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: POST /files: parsing multipart form: %v", err)
				internalServerError(w)
				return
			}
			uploadedFile, header, err := r.FormFile("uploadedFile")
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: POST /files: get form file: %v", err)
				internalServerError(w)
				return
			}
			destFile, err := os.Create(filepath.Join(cfg.Storage.UploadedFilesDir, header.Filename))
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: POST /files: creating file: %v", err)
				internalServerError(w)
				return
			}
			err = destFile.Chmod(0600)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: POST /files: changing file perms: %v", err)
				internalServerError(w)
				return
			}
			_, err = io.Copy(destFile, uploadedFile)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: POST /files: writing file: %v", err)
				internalServerError(w)
				return
			}

			tmpl, err := template.ParseFiles("templates/base.tmpl", "templates/upload-success.tmpl")
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: POST /files: parsing template: %v\n", err)
				internalServerError(w)
				return
			}
			err = tmpl.Execute(w, map[string]any{
				"Authenticated": sessionManager.GetBool(r.Context(), "authenticated"),
				"User":          sessionManager.GetString(r.Context(), "user"),

				"Link": fmt.Sprintf("%s://%s/u/%s", cfg.HTTP.Schema, cfg.HTTP.DomainName, header.Filename),
			})
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: POST /files: executing template: %v\n", err)
				internalServerError(w)
				return
			}
		} else {
			methodNotAllowed(w, []string{http.MethodGet, http.MethodPost, http.MethodHead})
		}
	})

	mux.HandleFunc("/t/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet || r.Method == http.MethodHead {
			tmpl, err := template.ParseFiles("templates/base.tmpl", "templates/t.tmpl")
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: GET /t/: parsing template: %v\n", err)
				internalServerError(w)
				return
			}
			filename := strings.TrimPrefix(r.URL.Path, "/t/")
			rawText, err := os.ReadFile(filepath.Join(cfg.Storage.UploadedFilesDir, filename))
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: GET /t/: reading file: %v\n", err)
				internalServerError(w)
				return
			}
			err = tmpl.Execute(w, map[string]any{
				"Authenticated": sessionManager.GetBool(r.Context(), "authenticated"),
				"User":          sessionManager.GetString(r.Context(), "user"),

				"Title": filename,
				"Text":  string(rawText),
			})
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: GET /t/: executing template: %v\n", err)
				internalServerError(w)
				return
			}
		} else {
			methodNotAllowed(w, []string{http.MethodGet, http.MethodHead})
		}
	})

	mux.HandleFunc("/u/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet || r.Method == http.MethodHead {
			tmpl, err := template.ParseFiles("templates/base.tmpl", "templates/u.tmpl")
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: GET /u/: parsing template: %v\n", err)
				internalServerError(w)
				return
			}
			safeFilename, err := sanitizeFilename(strings.TrimPrefix(r.URL.Path, "/u/"))
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: GET /u/: %v\n", err)
				// TODO: Return Bad Request and a proper error message.
				internalServerError(w)
				return
			}
			ctype := mime.TypeByExtension(filepath.Ext(safeFilename))
			err = tmpl.Execute(w, map[string]any{
				"Authenticated": sessionManager.GetBool(r.Context(), "authenticated"),
				"User":          sessionManager.GetString(r.Context(), "user"),

				"Title":       safeFilename,
				"MIMEType":    ctype,
				"FileType":    strings.Split(ctype, "/")[0],
				"RawfileLink": fmt.Sprintf("%s://%s/dl/%s", cfg.HTTP.Schema, cfg.HTTP.DomainName, safeFilename),
			})
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: GET /u/: executing template: %v\n", err)
				internalServerError(w)
				return
			}
		} else {
			methodNotAllowed(w, []string{http.MethodGet, http.MethodHead})
		}
	})

	mux.HandleFunc("/dl/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet || r.Method == http.MethodHead {
			safeFilename, err := sanitizeFilename(strings.TrimPrefix(r.URL.Path, "/dl/"))
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: GET /dl/: %v\n", err)
				// TODO: Return Bad Request and a proper error message.
				internalServerError(w)
				return
			}
			f, err := os.Open(filepath.Join(cfg.Storage.UploadedFilesDir, safeFilename))
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: GET /dl/: %v\n", err)
				// TODO: Return Bad Request and a proper error message.
				internalServerError(w)
				return
			}
			defer f.Close()
			http.ServeContent(w, r, safeFilename, time.Time{}, f)

		} else {
			methodNotAllowed(w, []string{http.MethodGet, http.MethodHead})
		}
	})

	fmt.Printf("Serving application on %s...\n", cfg.HTTP.Addr)
	log.Fatal(http.ListenAndServe(cfg.HTTP.Addr, sessionManager.LoadAndSave(mux)))
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

func unauthorized(w http.ResponseWriter) {
	w.WriteHeader(http.StatusUnauthorized)
	fmt.Fprintln(w, "You must be logged in to perform this action.")
}

// methodNotAllowed returns a Method Not Allowed response.
func methodNotAllowed(w http.ResponseWriter, allowedMethods []string) {
	w.Header().Add("Allow", strings.Join(allowedMethods, ", "))
	w.WriteHeader(http.StatusMethodNotAllowed)
	fmt.Fprintln(w, "Method Not Allowed")
}

// internalServerError returns an Internal Server Error response.
func internalServerError(w http.ResponseWriter) {
	w.WriteHeader(http.StatusInternalServerError)
	fmt.Fprintln(w, "Internal Server Error")
}

// sendTo redirects the user to the given webpage after a POST or PUT.
//
// See: https://developer.mozilla.org/en-US/docs/Web/HTTP/Status/303
func sendTo(w http.ResponseWriter, path string) {
	w.Header().Add("Location", path)
	w.WriteHeader(http.StatusSeeOther)
}

// authenticateUser authenticates the user and creates a new session.
func authenticateUser(r *http.Request, username, password string) error {
	db, err := sql.Open("sqlite3", cfg.Storage.DBPath)
	if err != nil {
		return err
	}
	defer db.Close()
	stmt, err := db.Prepare(`select salt, hash from users where username = ?`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	var saltHex, hashHex string
	err = stmt.QueryRow(username).Scan(&saltHex, &hashHex)
	if err != nil {
		return errors.New("user not found")
	}

	// Decode salt and saved hash to bytes.
	salt, err := hex.DecodeString(saltHex)
	if err != nil {
		return fmt.Errorf("decoding saved salt to bytes: %v", err)
	}
	savedHash, err := hex.DecodeString(hashHex)
	if err != nil {
		return fmt.Errorf("decoding saved argon2id hash to bytes: %v", err)
	}

	// Compute Argon2id key and compare with saved hash.
	key := argon2.IDKey([]byte(password), salt, 1, 60*1024, 1, 32)
	if !bytes.Equal(key, savedHash) {
		return errors.New("incorrect password")
	}

	// Save user authentication information to session.
	sessionManager.Put(r.Context(), "authenticated", true)
	sessionManager.Put(r.Context(), "user", username)

	return nil
}

func loggedIn(r *http.Request) bool {
	return sessionManager.GetBool(r.Context(), "authenticated")
}

type UploadedFile struct {
	dirEntry os.DirEntry
}

func (f UploadedFile) Name() string {
	return f.dirEntry.Name()
}

func (f UploadedFile) PageLink() string {
	return fmt.Sprintf("%s://%s/u/%s", cfg.HTTP.Schema, cfg.HTTP.DomainName, f.Name())
}

func (f UploadedFile) FileLink() string {
	return fmt.Sprintf("%s://%s/dl/%s", cfg.HTTP.Schema, cfg.HTTP.DomainName, f.Name())
}

func (f UploadedFile) MIMEType() string {
	return mime.TypeByExtension(filepath.Ext(f.Name()))
}

func (f UploadedFile) Type() string {
	return strings.Split(f.MIMEType(), "/")[0]
}

func getUploadedFiles(uploadedFilesDir string) ([]UploadedFile, error) {
	files, err := os.ReadDir(uploadedFilesDir)
	if err != nil {
		return nil, err
	}
	var uploadedFiles []UploadedFile
	for _, f := range files {
		uploadedFiles = append(uploadedFiles, UploadedFile{dirEntry: f})
	}
	return uploadedFiles, nil
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

// sanitizeFilename returns a filename safe to be served.
func sanitizeFilename(untrustedFilename string) (string, error) {
	// In case the filename has path separators, get only the last element of the path.
	base := filepath.Base(untrustedFilename)
	if base == "." || base == ".." {
		return "", fmt.Errorf("invalid filename: %q", untrustedFilename)
	}
	return base, nil
}
