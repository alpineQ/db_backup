package main

import (
	"bytes"
	"fmt"
	"html/template"
	"io/ioutil"
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
	r.HandleFunc("/db_backup/{container_name}/backup/", backup).Methods("POST")
	r.HandleFunc("/db_backup/{container_name}/restore/", restore)

	http.Handle("/", r)
	log.Fatal(http.ListenAndServe(":8080", nil))
}

func index(w http.ResponseWriter, r *http.Request) {
	indexData := make(map[string][]string)
	containerFolders, err := ioutil.ReadDir("backups")
	if err != nil {
		log.Fatal(err)
	}
	for _, containerFolder := range containerFolders {
		if !containerFolder.IsDir() {
			continue
		}
		backupFiles, err := ioutil.ReadDir("backups/" + containerFolder.Name())
		if err != nil {
			log.Fatal(err)
		}
		var containerBackups []string
		for _, backup := range backupFiles {
			containerBackups = append(containerBackups, backup.Name()[:len(backup.Name())-4])
		}
		indexData[containerFolder.Name()] = containerBackups
	}

	containers, err := client.ListContainers(
		docker.ListContainersOptions{Filters: map[string][]string{
			"status": []string{"running"}}})
	if err != nil {
		log.Printf("Error: %s", err)
	}
	for _, container := range containers {
		if len(indexData[container.Names[0][1:]]) == 0 {
			indexData[container.Names[0][1:]] = []string{}
		}
	}

	tpl.Execute(w, indexData)
}

func backup(w http.ResponseWriter, r *http.Request) {
	containerName := mux.Vars(r)["container_name"]
	containers, err := client.ListContainers(
		docker.ListContainersOptions{Filters: map[string][]string{
			"name": []string{containerName}}})
	if err != nil || len(containers) != 1 {
		log.Printf("Error: %s", err)
	}
	containerID := containers[0].ID
	backupName := time.Now().Format("02-01-2006-15:04")
	backupFilePath := "backups/" + containerName + "/" + backupName + ".tar"

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
	defer backupFile.Close()
	client.DownloadFromContainer(containerID, docker.DownloadFromContainerOptions{
		Path:         "/data/dump/" + backupName,
		OutputStream: backupFile,
	})
	http.Redirect(w, r, "/db_backup/", http.StatusFound)
}

func restore(w http.ResponseWriter, r *http.Request) {
	containerName := mux.Vars(r)["container_name"]
	containers, err := client.ListContainers(
		docker.ListContainersOptions{Filters: map[string][]string{
			"name": []string{containerName}}})
	if err != nil || len(containers) != 1 {
		log.Printf("Error: %s", err)
	}
	containerID := containers[0].ID

	backupDate := r.FormValue("date")
	if backupDate == "" {
		fmt.Fprint(w, ":(")
		return
	}
	backupFile, err := os.Open("backups/" + containerName + "/" + backupDate + ".tar")
	if err != nil {
		log.Printf("Error: %s", err)
		return
	}
	defer backupFile.Close()
	client.UploadToContainer(containerID, docker.UploadToContainerOptions{
		Path:        "/data/dump/",
		InputStream: backupFile,
	})
	execInfo, err := client.CreateExec(docker.CreateExecOptions{
		AttachStderr: true,
		AttachStdout: true,
		AttachStdin:  true,
		Tty:          false,
		Cmd:          []string{"mongorestore", "-v", "--dir=/data/dump/" + backupDate},
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
	http.Redirect(w, r, "/db_backup/", http.StatusFound)
}
