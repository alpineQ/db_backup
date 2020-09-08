package main

import (
	"log"
	"net/http"

	routes "github.com/alpineQ/db_backup/internal"
	"github.com/gorilla/mux"
)

func main() {
	r := mux.NewRouter()

	r.PathPrefix("/static/db_backup/").Handler(http.StripPrefix("/static/db_backup/", http.FileServer(http.Dir("static"))))
	r.HandleFunc("/db_backup/", routes.Index)
	r.HandleFunc("/db_backup/{container_name}/backup/", routes.Backup).Methods("POST")
	r.HandleFunc("/db_backup/{container_name}/restore/", routes.Restore)
	http.Handle("/", r)

	log.Fatal(http.ListenAndServe(":8080", nil))
}
