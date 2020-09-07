package main

import (
	"bytes"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	docker "github.com/fsouza/go-dockerclient"
	"github.com/gorilla/mux"
)

var tpl = template.Must(template.ParseFiles("index.html"))
var client, _ = docker.NewClientFromEnv()

func main() {
	r := mux.NewRouter()
	r.PathPrefix("/static/db_backup/").Handler(http.StripPrefix("/static/db_backup/", http.FileServer(http.Dir("static"))))

	r.HandleFunc("/db_backup/", index)
	r.HandleFunc("/db_backup/{container_id}/backup/", backup).Methods("POST")

	http.Handle("/", r)
	log.Fatal(http.ListenAndServe(":8080", nil))
}

func index(w http.ResponseWriter, r *http.Request) {
	containers, err := client.ListContainers(
		docker.ListContainersOptions{Filters: map[string][]string{
			"status": []string{"running"}}})
	if err != nil {
		panic(err)
	}
	tpl.Execute(w, containers)
}

func backup(w http.ResponseWriter, r *http.Request) {
	containerID := mux.Vars(r)["container_id"]
	containers, err := client.ListContainers(
		docker.ListContainersOptions{Filters: map[string][]string{
			"id": []string{containerID}}})
	if err != nil || len(containers) != 1 {
		panic(err)
	}
	containerName := containers[0].Names[0]
	backupFilePath := "backups" + containerName + "/backup.tar"
	backupName := time.Now().Format("02-01-2006")

	fmt.Fprint(w, containerID)
	execInfo, err := client.CreateExec(docker.CreateExecOptions{
		AttachStderr: true,
		AttachStdout: true,
		AttachStdin:  true,
		Tty:          false,
		Cmd:          []string{"mongodump", "--out=/data/dump/" + backupName},
		Container:    containerID,
	})
	if err != nil {
		log.Printf("Error: %s", err)
		return
	}
	var buffer bytes.Buffer
	if err = client.StartExec(execInfo.ID, docker.StartExecOptions{
		OutputStream: &buffer,
		ErrorStream:  &buffer,
	}); err != nil {
		log.Printf("Error: %s", err)
		return
	}
	fmt.Print(buffer.String())

	if err := os.MkdirAll(filepath.Dir(backupFilePath), 0770); err != nil {
		log.Printf("Error: %s", err)
		return
	}
	backupFile, err := os.Create(backupFilePath)
	if err != nil {
		log.Printf("Error: %s", err)
		return
	}
	client.DownloadFromContainer(containerID, docker.DownloadFromContainerOptions{
		Path:         "/data/dump/" + backupName,
		OutputStream: backupFile,
	})
}
