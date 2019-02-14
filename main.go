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
	"regexp"
	"strings"
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
	Commit string
	URL    string
	Port   string
	Status string
}

type giteaPushEventData struct {
	Secret string `json:"secret"`
	CommitID string `json:"after"`
	Repository struct {
		URL string `json:"clone_url"`
	} `json:"repository"`
	Commits []struct {
		Message string `json:"message"`
	} `json:"commits"`
}

func newWorkerID() string {
	return time.Now().Format("20060102150405.000000")
}

func getPortFromMessage(msg string) string {
	lines := strings.Split(msg, "\n")

	if len(lines) < 1 || strings.IndexByte(lines[0], ':') < 1 {
		return ""
	}

	re := regexp.MustCompile(`^([a-z0-9-]+)/([a-zA-Z0-9-_.]+)$`)

	port := strings.TrimSpace(lines[0][:strings.IndexByte(lines[0], ':')])

	if re.MatchString(port) {
		return port
	}

	return ""
}

func getCIInfoFromMessage(msg string) bool {
	lines := strings.Split(msg, "\n")

	for _, line := range lines {
		line = strings.ToLower(line)
		if strings.HasPrefix(line, "ci:") {
			if strings.Contains(line, "no") || strings.Contains(line, "false") {
				return false
			}
			if strings.Contains(line, "yes") || strings.Contains(line, "true") {
				return true
			}
		}
	}

	return false
}

func (c *controller) startWorker(workChan chan worker) {
	defer c.wg.Done()

	for {
		select {
		case wrk := <-workChan:
			env := append(os.Environ(),
				fmt.Sprintf("JOB_ID=%s", wrk.ID),
				fmt.Sprintf("COMMIT_ID=%s", wrk.Commit),
				fmt.Sprintf("REPO_URL=%s", wrk.URL),
				fmt.Sprintf("JOB_PORT=%s", wrk.Port),
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
			fmt.Fprint(w, "nothing to see here")
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

			if getCIInfoFromMessage(data.Commits[0].Message) == false {
				fmt.Fprint(w, "No build started")
				return
			}

			port := getPortFromMessage(data.Commits[0].Message)

			if port == "" {
				fmt.Fprint(w, "No category/port detected in commit message")
				return
			}

			select {
			case workChan <- worker{ID: newWorkerID(), Commit: data.CommitID, URL: data.Repository.URL, Port: port, Status: "init"}:
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
