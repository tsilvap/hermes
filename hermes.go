package main

import (
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
)

func main() {
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		tmpl, err := template.New("index.tmpl").ParseFiles("templates/index.tmpl")
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error parsing template: %v\n", err)
			internalServerError(w)
			return
		}
		err = tmpl.Execute(w, nil)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error executing template: %v\n", err)
			internalServerError(w)
			return
		}
	})

	addr := ":8080"
	fmt.Printf("Serving application on %s...\n", addr)
	log.Fatal(http.ListenAndServe(addr, nil))
}

// internalServerError returns an Internal Server Error page.
func internalServerError(w http.ResponseWriter) {
	w.WriteHeader(500)
	fmt.Fprintln(w, "Internal Server Error")
}
