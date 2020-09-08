package main

import (
	"log"
	"net/http"

	routes "github.com/alpineQ/db_backup/internal"
	config "github.com/alpineQ/db_backup/pkg"
	"github.com/gorilla/mux"
	"github.com/robfig/cron"
)

func main() {
	conf, err := config.Load("config.json")
	if err != nil {
		log.Panic(err)
		return
	}
	c := cron.New()
	for _, db := range conf.DBConfigs {
		c.AddFunc(db.BackupFreq, func() { routes.Backup(db.Name, db.BackupCMD) })
	}
	c.Start()
	defer c.Stop()

	r := mux.NewRouter()

	r.PathPrefix("/static/db_backup/").Handler(http.StripPrefix("/static/db_backup/", http.FileServer(http.Dir("static"))))
	r.HandleFunc("/db_backup/", routes.IndexRoute)
	r.HandleFunc("/db_backup/{container_name}/backup/", routes.BackupRoute).Methods("POST")
	r.HandleFunc("/db_backup/{container_name}/restore/", routes.RestoreRoute)
	http.Handle("/", r)

	log.Fatal(http.ListenAndServe(":8080", nil))
}
