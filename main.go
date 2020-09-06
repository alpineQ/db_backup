package main

import (
	"html/template"
	"log"
	"net/http"

	docker "github.com/fsouza/go-dockerclient"
)

var tpl = template.Must(template.ParseFiles("index.html"))

func main() {
	client, err := docker.NewClientFromEnv()
	if err != nil {
		panic(err)
	}

	fs := http.FileServer(http.Dir("static"))
	http.Handle("/static/db_backup/", http.StripPrefix("/static/db_backup/", fs))
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		containers, err := client.ListContainers(docker.ListContainersOptions{All: true})
		if err != nil {
			panic(err)
		}
		tpl.Execute(w, containers)
	})

	log.Fatal(http.ListenAndServe(":8080", nil))
}
