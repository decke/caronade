package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os/exec"
	"path"
	"time"
)

type controller struct {
	Workdir string
	Logdir  string
	Host    string
}

type worker struct {
	ID     string
	URL    string
	Status string
}

func newWorkerID() string {
	return time.Now().Format("20060102150405.000000")
}

func (c *controller) startWorker(workChan chan worker) {
	for {
		select {
		case wrk := <-workChan:
			cmd := exec.Cmd{Dir: c.Workdir, Path: "/usr/bin/make", Args: []string{"build"}}
			output, err := cmd.CombinedOutput()
			if err != nil {
				wrk.Status = "failed"
			} else {
				wrk.Status = "success"
			}
			ioutil.WriteFile(path.Join(c.Logdir, wrk.ID+".txt"), output, 0600)
		case <-time.After(time.Second * 1):
		}
	}
}

func (c *controller) startWebhook(workChan chan worker) {
	fs := http.FileServer(http.Dir("logs"))
	http.Handle("/logs/", http.StripPrefix("/logs/", fs))

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			fmt.Fprint(w, "Welcome to caronade")
		} else {
			select {
			case workChan <- worker{ID: newWorkerID(), URL: "your-url", Status: "init"}:
				fmt.Fprint(w, "Build started")
				return
			default:
				http.Error(w, "Build already in progress", http.StatusConflict)
				return
			}
		}
	})

	log.Printf("Listening on %s\n", c.Host)
	http.ListenAndServe(c.Host, nil)
}

func main() {
	var workdir string
	var logdir string
	var host string

	flag.StringVar(&workdir, "workdir", "work", "Working directory")
	flag.StringVar(&logdir, "logdir", "logs", "Buildlogs directory")
	flag.StringVar(&host, "host", ":3000", "Interface and port to listen on")
	flag.Parse()

	ctrl := controller{
		Workdir: workdir,
		Logdir:  logdir,
		Host:    host,
	}

	workChannel := make(chan worker, 1)

	go ctrl.startWorker(workChannel)
	ctrl.startWebhook(workChannel)
}
