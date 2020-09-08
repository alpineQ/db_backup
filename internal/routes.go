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
	"strings"
	"time"

	config "github.com/alpineQ/db_backup/pkg"
	jwt "github.com/dgrijalva/jwt-go"
	docker "github.com/fsouza/go-dockerclient"
	"github.com/gorilla/mux"
)

var client, _ = docker.NewClientFromEnv()
var tpl = template.Must(template.ParseFiles("templates/index.html"))

// IndexData Структура предоставляемая Index для сборки HTML страницы
type IndexData struct {
	DBInfos  []DBInfo
	Username string
	Group    string
}

// DBInfo Информация о конкретной БД
type DBInfo struct {
	Name    string
	Backups []string
	Status  string
}

// IndexRoute Основная страница приложения
func IndexRoute(w http.ResponseWriter, r *http.Request) {
	var dbInfos = []DBInfo{}
	containerFolders, err := ioutil.ReadDir("backups")
	if err != nil {
		log.Fatal(err)
	}
	for _, dbConfig := range config.Config.DBConfigs {
		dbInfos = append(dbInfos, DBInfo{Name: dbConfig.Name, Status: "Down"})
		for _, containerFolder := range containerFolders {
			if !containerFolder.IsDir() {
				continue
			}
			if dbConfig.Name == containerFolder.Name() {
				backupFiles, err := ioutil.ReadDir("backups/" + containerFolder.Name())
				if err != nil {
					log.Fatal(err)
				}
				var containerBackups []string
				for _, backup := range backupFiles {
					containerBackups = append(containerBackups, backup.Name()[:len(backup.Name())-4])
				}
				for dbIndex, db := range dbInfos {
					if db.Name == dbConfig.Name {
						dbInfos[dbIndex].Backups = containerBackups
						break
					}
				}
			}
		}
	}

	containers, err := client.ListContainers(docker.ListContainersOptions{Filters: map[string][]string{}})
	if err != nil {
		log.Printf("Error: %s", err)
	}
	for _, container := range containers {
		for dbIndex, db := range dbInfos {
			if container.Names[0][1:] == db.Name {
				dbInfos[dbIndex].Status = container.Status
			}
		}
	}
	username, group := GetAuthData(r)

	tpl.Execute(w, IndexData{DBInfos: dbInfos, Username: username, Group: group})
}

// BackupRoute End-point резервного копирования
func BackupRoute(w http.ResponseWriter, r *http.Request) {
	containerName := mux.Vars(r)["container_name"]
	for _, db := range config.Config.DBConfigs {
		if db.Name == containerName {
			if err := Backup(db.Name, db.BackupCMD); err != nil {
				log.Panic(err)
				return
			}
			http.Redirect(w, r, "/db_backup/", http.StatusFound)
			return
		}
	}
	fmt.Fprintf(w, "DB not found!")
}

// RestoreRoute End-point восстановления резервной копии
func RestoreRoute(w http.ResponseWriter, r *http.Request) {
	containerName := mux.Vars(r)["container_name"]
	backupDate := r.FormValue("date")
	if backupDate == "" {
		fmt.Fprint(w, ":(")
		return
	}
	for _, db := range config.Config.DBConfigs {
		if db.Name == containerName {
			if err := Restore(db.Name, db.RestoreCMD, backupDate); err != nil {
				log.Panic(err)
				return
			}
			http.Redirect(w, r, "/db_backup/", http.StatusFound)
			return
		}
	}
	fmt.Fprintf(w, "DB not found!")
}

func getKeyByID(token *jwt.Token) (interface{}, error) {
	if _, ok := token.Method.(*jwt.SigningMethodRSA); !ok {
		return nil, fmt.Errorf("Unexpected signing method: %v", token.Header["alg"])
	}
	keyID := token.Header["kid"]
	if keyID == "" {
		return "", errors.New("Invalid JWT")
	}
	authURL := os.Getenv("AUTH_URL")
	if authURL == "" {
		authURL = "https://sms.gitwork.ru/auth"
	}
	resp, err := http.Get(authURL + "/public_key/" + fmt.Sprintf("%v", keyID))
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

// Backup Создание резервной копии
func Backup(containerName string, backupCmd []string) error {
	containers, err := client.ListContainers(
		docker.ListContainersOptions{Filters: map[string][]string{
			"name": []string{containerName}}})
	if err != nil || len(containers) != 1 {
		return err
	}
	containerID := containers[0].ID
	backupName := time.Now().Format("02-01-2006-15:04")
	backupFilePath := "backups/" + containerName + "/" + backupName + ".tar"

	for index, elem := range backupCmd {
		if strings.Contains(elem, "$date") {
			backupCmd[index] = strings.Replace(backupCmd[index], "$date", backupName, -1)
		}
	}

	execInfo, err := client.CreateExec(docker.CreateExecOptions{
		AttachStderr: true,
		AttachStdout: true,
		AttachStdin:  true,
		Tty:          false,
		Cmd:          backupCmd,
		Container:    containerID,
	})
	if err != nil {
		return err
	}
	var buffer bytes.Buffer
	if err = client.StartExec(execInfo.ID, docker.StartExecOptions{
		OutputStream: &buffer,
		ErrorStream:  &buffer,
	}); err != nil {
		return err
	}
	fmt.Printf(buffer.String())

	if err := os.MkdirAll(filepath.Dir(backupFilePath), 0770); err != nil {
		return err
	}
	backupFile, err := os.Create(backupFilePath)
	if err != nil {
		return err
	}
	defer backupFile.Close()
	client.DownloadFromContainer(containerID, docker.DownloadFromContainerOptions{
		Path:         "/data/dump/" + backupName,
		OutputStream: backupFile,
	})
	return nil
}

// Restore Восстановление резервной копии
func Restore(containerName string, restoreCmd []string, backupName string) error {
	containers, err := client.ListContainers(
		docker.ListContainersOptions{Filters: map[string][]string{
			"name": []string{containerName}}})
	if err != nil || len(containers) != 1 {
		log.Printf("Error: %s", err)
	}
	containerID := containers[0].ID

	for index, elem := range restoreCmd {
		if strings.Contains(elem, "$date") {
			restoreCmd[index] = strings.Replace(restoreCmd[index], "$date", backupName, -1)
		}
	}

	backupFile, err := os.Open("backups/" + containerName + "/" + backupName + ".tar")
	if err != nil {
		return err
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
		Cmd:          restoreCmd,
		Container:    containerID,
	})
	if err != nil {
		return err
	}
	var buffer bytes.Buffer
	if err = client.StartExec(execInfo.ID, docker.StartExecOptions{
		OutputStream: &buffer,
		ErrorStream:  &buffer,
	}); err != nil {
		return err
	}
	fmt.Printf(buffer.String())
	return nil
}
