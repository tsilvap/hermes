package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/tsilvap/hermes/internal/models"
)

func getTestServer() *httptest.Server {
	logger := NewStderrLogger()
	app := App{Logger: logger, uploadedFiles: &models.UploadedFileModel{DB: nil}}
	r := appRouter(app)
	return httptest.NewServer(r)
}

func Test200(t *testing.T) {
	s := getTestServer()
	defer s.Close()

	r, err := s.Client().Get(s.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	got, want := r.StatusCode, http.StatusOK
	if got != want {
		t.Errorf("r.StatusCode = %d, want %d", got, want)
	}
}

func Test404(t *testing.T) {
	s := getTestServer()
	defer s.Close()

	testCases := []struct {
		Path string
	}{
		{"/notexistent"},
		{"/t"}, {"/t/"}, {"/t/notexistent"},
		{"/u"}, {"/u/"}, {"/u/notexistent"},
		{"/dl"}, {"/dl/"}, {"/dl/notexistent"},
	}
	for _, tc := range testCases {
		t.Run("GET "+tc.Path, func(t *testing.T) {
			r, err := s.Client().Get(s.URL + tc.Path)
			if err != nil {
				t.Fatal(err)
			}
			got, want := r.StatusCode, http.StatusNotFound
			if got != want {
				t.Errorf("r.StatusCode = %d, want %d", got, want)
			}
		})
	}
}

func Test405(t *testing.T) {
	s := getTestServer()
	defer s.Close()

	testCases := []struct {
		Method        string
		Path          string
		ExpectedAllow string
	}{
		{"GET", "/logout", "POST"},
		{"DELETE", "/text", "GET, POST"},
	}
	for _, tc := range testCases {
		t.Run(fmt.Sprintf("%s %s", tc.Method, tc.Path), func(t *testing.T) {
			var r *http.Response
			var err error
			switch tc.Method {
			case "GET":
				r, err = s.Client().Get(s.URL + tc.Path)
			case "DELETE":
				req, reqErr := http.NewRequest("DELETE", s.URL+tc.Path, nil)
				if reqErr != nil {
					t.Fatal(err)
				}
				r, err = s.Client().Do(req)
			default:
				r, err = s.Client().Get(s.URL + tc.Path)
			}
			if err != nil {
				t.Fatal(err)
			}
			sc, scWant := r.StatusCode, http.StatusMethodNotAllowed
			if sc != scWant {
				t.Errorf("r.StatusCode = %d, want %d", sc, scWant)
			}
			slices.Sort(r.Header.Values("Allow"))
			allow, allowWant := strings.Join(r.Header.Values("Allow"), ", "), tc.ExpectedAllow
			if allow != allowWant {
				t.Errorf("Allow header = %q, want %q", allow, allowWant)
			}
		})
	}
}
