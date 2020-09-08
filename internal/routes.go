package routes

import (
	"bytes"
	"errors"
	"fmt"
	"html/template"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	jwt "github.com/dgrijalva/jwt-go"
	docker "github.com/fsouza/go-dockerclient"
	"github.com/gorilla/mux"
)

var client, _ = docker.NewClientFromEnv()
var tpl = template.Must(template.ParseFiles("templates/index.html"))

// IndexData Структура предоставляемая Index для сборки HTML страницы
type IndexData struct {
	DBInfo   map[string][]string
	Username string
	Group    string
}

// Index Основная страница приложения
func Index(w http.ResponseWriter, r *http.Request) {
	dbInfo := make(map[string][]string)
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
		dbInfo[containerFolder.Name()] = containerBackups
	}

	containers, err := client.ListContainers(
		docker.ListContainersOptions{
			Filters: map[string][]string{
				"status": []string{"running"}}})
	if err != nil {
		log.Printf("Error: %s", err)
	}
	for _, container := range containers {
		if len(dbInfo[container.Names[0][1:]]) == 0 {
			dbInfo[container.Names[0][1:]] = []string{}
		}
	}

	username, group := GetAuthData(r)

	tpl.Execute(w, IndexData{DBInfo: dbInfo, Username: username, Group: group})
}

// Backup End-point резервного копирования
func Backup(w http.ResponseWriter, r *http.Request) {
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

// Restore End-point восстановления резервной копии
func Restore(w http.ResponseWriter, r *http.Request) {
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
	http.Redirect(w, r, "/db_backup/", http.StatusFound)
}

func getKeyByID(token *jwt.Token) (interface{}, error) {
	if _, ok := token.Method.(*jwt.SigningMethodRSA); !ok {
		return nil, fmt.Errorf("Unexpected signing method: %v", token.Header["alg"])
	}
	keyID := token.Header["kid"]
	if keyID == "" {
		return "", errors.New("Invalid JWT")
	}

	resp, err := http.Get("http://gateway/auth/public_key/" + fmt.Sprintf("%v", keyID))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	publicKey, err := jwt.ParseRSAPublicKeyFromPEM(body)
	if err != nil {
		return "", err
	}
	return publicKey, nil
}

// GetAuthData Функция получения данных авторизации из запроса
func GetAuthData(r *http.Request) (string, string) {
	cookie, err := r.Cookie("user_jwt")
	if err != nil {
		log.Fatal(err)
		return "", ""
	}

	token, err := jwt.Parse(cookie.Value, getKeyByID)
	if err != nil {
		log.Fatal(err)
		return "", ""
	}

	if claims, ok := token.Claims.(jwt.MapClaims); ok && token.Valid {
		return fmt.Sprintf("%v", claims["username"]), fmt.Sprintf("%v", claims["group"])
	}
	return "", ""
}
