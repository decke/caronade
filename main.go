package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path"
	"sync"
	"time"
)

type controller struct {
	wg      *sync.WaitGroup
	Workdir string
	Logdir  string
	Host    string
	Secret  string
}

type worker struct {
	ID     string
	URL    string
	Status string
}

type giteaPushEventData struct {
	Secret string `json:"secret"`
	CommitID string `json:"after"`
	Repository struct {
		URL string `json:"clone_url"`
	} `json:"repository"`
}

func (c *controller) startWorker(workChan chan worker) {
	defer c.wg.Done()

	for {
		select {
		case wrk := <-workChan:
			env := append(os.Environ(),
				fmt.Sprintf("COMMIT_ID=%s", wrk.ID),
				fmt.Sprintf("REPO_URL=%s", wrk.URL),
			)
			cmd := exec.Cmd{Dir: c.Workdir, Env: env, Path: "/usr/bin/make", Args: []string{"build"}}
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
	defer c.wg.Done()

	fs := http.FileServer(http.Dir("logs"))
	http.Handle("/logs/", http.StripPrefix("/logs/", fs))

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			fmt.Fprint(w, "Welcome to caronade")
		} else {
			payload, err := ioutil.ReadAll(r.Body)
			if err != nil {
				http.Error(w, "Internal Error", http.StatusInternalServerError)
				return
			}

			if r.Header.Get("X-Gitea-Event") != "push" {
				http.Error(w, "Invalid webhook", http.StatusBadRequest)
				return
			}

			data := giteaPushEventData{}
			if err = json.Unmarshal(payload, &data); err != nil {
				http.Error(w, "Failed to parse webhook data", http.StatusBadRequest)
				return
			}

			if c.Secret != "" {
				if data.Secret != c.Secret {
					http.Error(w, "Invalid secret", http.StatusBadRequest)
					return
				}
			}

			select {
			case workChan <- worker{ID: data.CommitID, URL: data.Repository.URL, Status: "init"}:
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
	var secret string

	flag.StringVar(&workdir, "workdir", "work", "Working directory")
	flag.StringVar(&logdir, "logdir", "logs", "Buildlogs directory")
	flag.StringVar(&host, "host", ":3000", "Interface and port to listen on")
	flag.StringVar(&secret, "secret", "", "Webhook secret")
	flag.Parse()

	wg := sync.WaitGroup{}

	ctrl := controller{
		wg:      &wg,
		Workdir: workdir,
		Logdir:  logdir,
		Host:    host,
		Secret:  secret,
	}

	workChannel := make(chan worker, 1)

	wg.Add(2)

	go ctrl.startWorker(workChannel)
	go ctrl.startWebhook(workChannel)

	wg.Wait()
}
