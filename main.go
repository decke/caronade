package main

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha1"
	"crypto/tls"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"gopkg.in/yaml.v2"
)

type controller struct {
	wg     *sync.WaitGroup
	cfg    *Config
}

type Queue struct {
	Name      string
	Recipe    string
	Environment map[string]string
	queue     chan worker
}

type Config struct {
	Workdir  string
	Logdir   string
	Server struct {
		Host     string
		BaseURL  string
		TLScert  string
		TLSkey   string
	}
	Webhook struct {
		Secret   string
	}
	Repository struct {
		APIURL   string
		APIToken string
	}
	Queues []Queue
	DefaultQueues []string `yaml:"default_queues"`
}

type worker struct {
	ID           string
	Status       string
	Queue        Queue
	Port         string
	Commit       string
	RepoURL      string
	RepoName     string
	RepoFullName string
}

type gitPushEventData struct {
	Secret     string `json:"secret"`
	CommitID   string `json:"after"`
	Repository struct {
		Name     string `json:"name"`
		FullName string `json:"full_name"`
		URL      string `json:"clone_url"`
	} `json:"repository"`
	Commits []struct {
		Message string `json:"message"`
	} `json:"commits"`
}

func calcSignature(payload *[]byte, secret string) string {
	mac := hmac.New(sha1.New, []byte(secret))
	mac.Write(*payload)

	return fmt.Sprintf("sha1=%x", mac.Sum(nil))
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

func (c *controller) getQueueInfoFromMessage(msg string) []Queue {
	queues := make([]Queue, 0)
	lines := strings.Split(msg, "\n")

	for _, line := range lines {
		line = strings.ToLower(line)
		if strings.HasPrefix(line, "ci:") {
			if strings.Contains(line, "no") || strings.Contains(line, "false") {
				return queues
			}
			if strings.Contains(line, "yes") || strings.Contains(line, "true") {
				for i := range(c.cfg.Queues) {
					queues = append(queues, c.cfg.Queues[i])
				}
				return queues
			}
		}
	}

	for _, name := range(c.cfg.DefaultQueues) {
		q := c.getQueueByName(name)
		if q.Name == "" {
			continue
		}
		queues = append(queues, q)
	}

	return queues
}

func (c *controller) getQueueByName(name string) Queue {
	for i := range(c.cfg.Queues) {
		if c.cfg.Queues[i].Name == name {
			return c.cfg.Queues[i]
		}
	}

	return Queue{}
}

func (c *controller) sendStatusUpdate(wrk worker) error {
	target := ""

	if wrk.Status != "pending" {
		target = fmt.Sprintf("%s/logs/%s.txt", c.cfg.Server.BaseURL, wrk.ID)
	}

	url := fmt.Sprintf("%s/repos/%s/statuses/%s?access_token=%s",
		c.cfg.Repository.APIURL, wrk.RepoFullName, wrk.Commit, c.cfg.Repository.APIToken)

	jsonValue, _ := json.Marshal(map[string]string{
		"state": wrk.Status,
		"target_url": target,
		"context": wrk.Queue.Name,
	})

	_, err := http.Post(url, "application/json", bytes.NewBuffer(jsonValue))

	return err
}

func (c *controller) startWorker(workChan chan worker) {
	defer c.wg.Done()

	for {
		select {
		case wrk := <-workChan:
			queue := c.getQueueByName(wrk.Queue.Name)

			log.Printf("ID %s started on %s\n", wrk.ID, queue.Name)
			c.sendStatusUpdate(wrk)

			env := append(os.Environ(),
				fmt.Sprintf("JOB_ID=%s", wrk.ID),
				fmt.Sprintf("COMMIT_ID=%s", wrk.Commit),
				fmt.Sprintf("REPO_URL=%s", wrk.RepoURL),
				fmt.Sprintf("JOB_PORT=%s", wrk.Port),
			)

			for k, v := range(queue.Environment) {
				env = append(env, fmt.Sprintf("%s=%s", k, v))
			}

			workdir := strings.Replace(queue.Name, "/", "", -1)
			workdir = strings.Replace(workdir, " ", "", -1)
			workdir = path.Join(c.cfg.Workdir, workdir)
			os.MkdirAll(workdir, os.ModePerm)

			cmd := exec.Cmd{
				Dir: workdir,
				Env: env,
				Path: "/usr/bin/make",
				Args: []string{
					"make",
					"-C", workdir,
					"-f", fmt.Sprintf("%s.mk", queue.Recipe),
					"-I", c.cfg.Workdir,
					"all",
				},
			}
			output, err := cmd.CombinedOutput()
			if err != nil {
				wrk.Status = "failure"
			} else {
				wrk.Status = "success"
			}
			ioutil.WriteFile(path.Join(c.cfg.Logdir, wrk.ID+".txt"), output, 0600)

			log.Printf("ID %s finished %s\n", wrk.ID, wrk.Status)
			c.sendStatusUpdate(wrk)

		case <-time.After(time.Second * 1):
		}
	}
}

func (c *controller) startWebhook() {
	defer c.wg.Done()

	mux := http.NewServeMux()

	fs := http.FileServer(http.Dir("logs"))
	mux.Handle("/logs/", http.StripPrefix("/logs/", fs))

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			fmt.Fprint(w, "nothing to see here")
			return
		} else {
			payload, err := ioutil.ReadAll(r.Body)
			if err != nil {
				http.Error(w, "Internal Error", http.StatusInternalServerError)
				return
			}

			if r.Header.Get("X-GitHub-Event") != "" {
				if r.Header.Get("X-GitHub-Event") != "push" {
					http.Error(w, "Invalid webhook", http.StatusBadRequest)
					return
				}
			}

			data := gitPushEventData{}
			if err = json.Unmarshal(payload, &data); err != nil {
				http.Error(w, "Failed to parse webhook data", http.StatusBadRequest)
				return
			}

			if c.cfg.Webhook.Secret != "" {
				if r.Header.Get("X-Hub-Signature") != "" {
					if calcSignature(&payload, c.cfg.Webhook.Secret) != r.Header.Get("X-Hub-Signature") {
						http.Error(w, "Invalid secret", http.StatusBadRequest)
						return
					}
				} else {
					if data.Secret != c.cfg.Webhook.Secret {
						http.Error(w, "Invalid secret", http.StatusBadRequest)
						return
					}
				}
			}

			port := getPortFromMessage(data.Commits[0].Message)

			if port == "" {
				fmt.Fprint(w, "No category/port detected in commit message")
				return
			}

			cnt := 0
			for _, q := range(c.getQueueInfoFromMessage(data.Commits[0].Message)) {
				job := worker {
					ID:           newWorkerID(),
					Status:       "pending",
					Queue:        q,
					Port:         port,
					Commit:       data.CommitID,
					RepoURL:      data.Repository.URL,
					RepoName:     data.Repository.Name,
					RepoFullName: data.Repository.FullName,
				}

				select {
				case q.queue <- job:
					cnt++
					log.Printf("%s Port %s queued on %s (pos %d)\n", job.ID, job.Port, q.Name, len(q.queue))
				default:
					log.Printf("%s Queue limit reached on queue %s\n", job.ID, q.Name)
				}
			}
			fmt.Fprintf(w, "%d Jobs queued", cnt)
		}
	})

	var err error
	if c.cfg.Server.TLScert != "" && c.cfg.Server.TLSkey != "" {
		cfg := &tls.Config{
			MinVersion:               tls.VersionTLS12,
			CurvePreferences:         []tls.CurveID{tls.CurveP521, tls.CurveP384, tls.CurveP256},
			PreferServerCipherSuites: true,
			CipherSuites: []uint16{
				tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
				tls.TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA,
				tls.TLS_RSA_WITH_AES_256_GCM_SHA384,
				tls.TLS_RSA_WITH_AES_256_CBC_SHA,
			},
		}
		srv := &http.Server{
			Addr:         c.cfg.Server.Host,
			Handler:      mux,
			TLSConfig:    cfg,
			TLSNextProto: make(map[string]func(*http.Server, *tls.Conn, http.Handler), 0),
		}

		log.Printf("Listening on %s (https)\n", c.cfg.Server.Host)
		err = srv.ListenAndServeTLS(c.cfg.Server.TLScert, c.cfg.Server.TLSkey)
	} else {
		srv := &http.Server{
			Addr:         c.cfg.Server.Host,
			Handler:      mux,
		}

		log.Printf("Listening on %s (http)\n", c.cfg.Server.Host)
		err = srv.ListenAndServe()
	}

	if err != nil {
		log.Printf("Listen failed: %s\n", err)
	}
}

func ParseConfig(file string) Config {
	f, err := os.Open(file)
	if err != nil {
		log.Fatalf("Error: %v", err)
	}
	defer f.Close()

	dec := yaml.NewDecoder(f)

	cfg := Config{}
	err = dec.Decode(&cfg)
	if err != nil {
		log.Fatalf("Error: %v", err)
	}

	cfg.Workdir, _ = filepath.Abs(cfg.Workdir)
	cfg.Logdir, _ = filepath.Abs(cfg.Logdir)
	cfg.Server.BaseURL = strings.TrimSuffix(cfg.Server.BaseURL, "/")
	cfg.Repository.APIURL = strings.TrimSuffix(cfg.Repository.APIURL, "/")

	return cfg
}

func main() {
	var cfgfile string

	flag.StringVar(&cfgfile, "config", "caronade.yaml", "Path to config file")
	flag.Parse()

	cfg := ParseConfig(cfgfile)

	wg := sync.WaitGroup{}

	for i := range(cfg.Queues) {
		log.Printf("Adding queue %s\n", cfg.Queues[i].Name)
		cfg.Queues[i].queue = make(chan worker, 10)
	}

	ctrl := controller{
		wg:     &wg,
		cfg:    &cfg,
	}

	for i := range(cfg.Queues) {
		wg.Add(1)
		go ctrl.startWorker(cfg.Queues[i].queue)
	}

	wg.Add(1)
	go ctrl.startWebhook()

	wg.Wait()
}
