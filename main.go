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
			ioutil.WriteFile(path.Join(c.Workdir, wrk.ID + ".txt"), output, 0600)
		case <-time.After(time.Second * 1):
		}
	}
}

func (c *controller) startWebhook(workChan chan worker) {
	http.HandleFunc("/webhook/", func(rw http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			select {
			case workChan <- worker{ID: newWorkerID(), URL: "your-url", Status: "init"}:
				fmt.Fprint(rw, "Build started")
				return
			default:
				http.Error(rw, "Build already in progress", http.StatusConflict)
				return
			}
		} else {
			fmt.Fprint(rw, "Not implemented")
		}
	})
	log.Printf("Listening on %s\n", c.Host)
	http.ListenAndServe(c.Host, nil)
}

func main() {
	var workdir string
	var host string

	flag.StringVar(&workdir, "workdir", "wrkdir", "Working directory")
	flag.StringVar(&host, "host", ":3000", "Interface and port to listen on")
	flag.Parse()

	ctrl := controller{
		Workdir: workdir,
 		Host:    host,
	}

	workChannel := make(chan worker, 1)

	go ctrl.startWorker(workChannel)
	ctrl.startWebhook(workChannel)
}
