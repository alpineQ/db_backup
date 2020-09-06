package main

import (
	"html/template"
	"log"
	"net/http"

	docker "github.com/fsouza/go-dockerclient"
	"github.com/gorilla/mux"
)

var tpl = template.Must(template.ParseFiles("index.html"))
var client, _ = docker.NewClientFromEnv()

func main() {
	r := mux.NewRouter()
	r.PathPrefix("/static/db_backup/").Handler(http.StripPrefix("/static/db_backup/", http.FileServer(http.Dir("static"))))

	r.HandleFunc("/", index)

	http.Handle("/", r)
	log.Fatal(http.ListenAndServe(":8080", nil))
}

func index(w http.ResponseWriter, r *http.Request) {
	containers, err := client.ListContainers(docker.ListContainersOptions{All: true})
	if err != nil {
		panic(err)
	}
	tpl.Execute(w, containers)
}
