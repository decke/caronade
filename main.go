package main

import (
	"bufio"
	"bytes"
	"crypto/hmac"
	"crypto/sha1"
	"crypto/tls"
	"encoding/json"
	"flag"
	"fmt"
	"html/template"
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

	"github.com/NYTimes/gziphandler"
	"gopkg.in/yaml.v2"
)

type controller struct {
	wg     *sync.WaitGroup
	cfg    *config
	queues map[string]*queue
}

type config struct {
	Workdir   string
	Logdir    string
	Staticdir string
	Tmpldir   string
	Server    struct {
		Host    string
		BaseURL string
		TLScert string
		TLSkey  string
	}
	Webhook struct {
		Secret string
	}
	Repository struct {
		APIURL   string
		APIToken string
	}
	Queues        []queue
	DefaultQueues []string `yaml:"default_queues"`
}

type queue struct {
	Name        string
	Recipe      string
	Environment map[string]string
	Workdir     string
	PathMatch   string
	queue       chan *job
}

type job struct {
	ID        string
	Startdate time.Time
	Enddate   time.Time
	Build     map[string]*build
	PushEvent gitPushEventData
}

type build struct {
	ID        string
	Queue     string
	Status    string
	Logfile   string
	Startdate time.Time
	Enddate   time.Time
}

type gitPushEventData struct {
	Secret     string `json:"secret"`
	CommitID   string `json:"after"`
	Repository struct {
		Name     string `json:"name"`
		FullName string `json:"full_name"`
		HTMLURL  string `json:"html_url"`
		CloneURL string `json:"clone_url"`
	} `json:"repository"`
	Commits []struct {
		Message string `json:"message"`
		URL     string `json:"url"`
		Author  struct {
			Name     string `json:"name"`
			EMail    string `json:"email"`
			Username string `json:"username"`
		} `json:"author"`
		Added    []string `json:"added"`
		Removed  []string `json:"removed"`
		Modified []string `json:"modified"`
	} `json:"commits"`
}

func calcSignature(payload *[]byte, secret string) string {
	mac := hmac.New(sha1.New, []byte(secret))
	mac.Write(*payload)

	return fmt.Sprintf("sha1=%x", mac.Sum(nil))
}

func (c *controller) matchQueues(data gitPushEventData) []queue {
	queues := make([]queue, 0)

NEXTQUEUE:
	for i := range c.cfg.Queues {
		re := regexp.MustCompile(c.cfg.Queues[i].PathMatch)

		for commit := range data.Commits {
			for _, file := range data.Commits[commit].Added {
				if re.MatchString(file) {
					queues = append(queues, c.cfg.Queues[i])
					continue NEXTQUEUE
				}
			}

			for _, file := range data.Commits[commit].Modified {
				if re.MatchString(file) {
					queues = append(queues, c.cfg.Queues[i])
					continue NEXTQUEUE
				}
			}
		}
	}

	return queues
}

func (j *job) StartDate() string {
	return j.Startdate.Format(time.RFC850)
}

func (b *build) LogfileContent() string {
	raw, err := ioutil.ReadFile(b.Logfile)
	if err != nil {
		return ""
	}

	return string(raw)
}

func (c *controller) renderBuildTemplate(j *job) {
	tmpl, err := template.ParseFiles(path.Join(c.cfg.Tmpldir, "index.html"))
	if err != nil {
		log.Printf("Failed parsing template: %v", err)
		return
	}

	outfile, _ := os.Create(path.Join(c.cfg.Logdir, j.ID, "index.html"))
	defer outfile.Close()

	writer := bufio.NewWriter(outfile)
	err = tmpl.Execute(writer, &j)
	if err != nil {
		log.Printf("Failed executing template: %v", err)
		return
	}
	writer.Flush()
	outfile.Sync()
}

func (c *controller) sendStatusUpdate(j *job, b *build) error {
	target := ""

	if b.Status != "pending" {
		target = fmt.Sprintf("%s/builds/%s/", c.cfg.Server.BaseURL, j.ID)
	}

	url := fmt.Sprintf("%s/repos/%s/statuses/%s?access_token=%s",
		c.cfg.Repository.APIURL, j.PushEvent.Repository.FullName, j.PushEvent.CommitID, c.cfg.Repository.APIToken)

	jsonValue, _ := json.Marshal(map[string]string{
		"state":      b.Status,
		"target_url": target,
		"context":    b.Queue,
	})

	_, err := http.Post(url, "application/json", bytes.NewBuffer(jsonValue))

	return err
}

func (c *controller) startWorker(q *queue) {
	defer c.wg.Done()

	for {
		var j *job
		select {
		case j = <-q.queue:
			b := j.Build[q.Name]
			b.Startdate = time.Now()

			log.Printf("ID %s started on %s\n", j.ID, q.Name)
			c.sendStatusUpdate(j, b)

			env := append(os.Environ(),
				fmt.Sprintf("JOB_ID=%s", j.ID),
				fmt.Sprintf("COMMIT_ID=%s", j.PushEvent.CommitID),
				fmt.Sprintf("REPO_URL=%s", j.PushEvent.Repository.CloneURL),
			)

			for k, v := range q.Environment {
				env = append(env, fmt.Sprintf("%s=%s", k, v))
			}

			os.MkdirAll(q.Workdir, os.ModePerm)

			cmd := exec.Cmd{
				Dir:  q.Workdir,
				Env:  env,
				Path: "/usr/bin/make",
				Args: []string{
					"make",
					"-C", q.Workdir,
					"-f", fmt.Sprintf("%s.mk", q.Recipe),
					"-I", c.cfg.Workdir,
					"all",
				},
			}
			output, err := cmd.CombinedOutput()
			if err != nil {
				b.Status = "failure"
			} else {
				b.Status = "success"
			}
			b.Enddate = time.Now()
			j.Enddate = time.Now()

			b.Logfile = path.Join(c.cfg.Logdir, j.ID, b.ID+".log")
			os.MkdirAll(filepath.Dir(b.Logfile), os.ModePerm)
			ioutil.WriteFile(b.Logfile, output, 0600)

			log.Printf("ID %s on %s finished %s\n", j.ID, q.Name, b.Status)
			c.sendStatusUpdate(j, b)
			c.renderBuildTemplate(j)

		case <-time.After(time.Second * 1):
		}
	}
}

func (c *controller) handleWebhook(w http.ResponseWriter, r *http.Request) {
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

	job := job{
		ID:        time.Now().Format("20060102150405.000"),
		Startdate: time.Now(),
		Build:     make(map[string]*build),
		PushEvent: data,
	}

	cnt := 0
	for _, q := range c.matchQueues(data) {
		b := build{
			ID:     fmt.Sprintf("%03d", cnt+1),
			Queue:  q.Name,
			Status: "pending",
		}
		job.Build[q.Name] = &b

		select {
		case q.queue <- &job:
			cnt++
			log.Printf("ID %s queued on %s (pos %d)\n", job.ID, q.Name, len(q.queue))
		default:
			log.Printf("ID %s Queue limit reached on queue %s\n", job.ID, q.Name)
		}
	}
	fmt.Fprintf(w, "ID %s has %d Jobs queued", job.ID, cnt)
}

func (c *controller) startHTTPD() {
	defer c.wg.Done()

	mux := http.NewServeMux()

	staticHandlerGz := gziphandler.GzipHandler(http.StripPrefix("/static/", http.FileServer(http.Dir(c.cfg.Staticdir))))
	mux.Handle("/static/", staticHandlerGz)

	buildHandlerGz := gziphandler.GzipHandler(http.StripPrefix("/builds/", http.FileServer(http.Dir(c.cfg.Logdir))))
	mux.Handle("/builds/", buildHandlerGz)

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			fmt.Fprint(w, "nothing to see here")
		} else {
			c.handleWebhook(w, r)
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
			Addr:    c.cfg.Server.Host,
			Handler: mux,
		}

		log.Printf("Listening on %s (http)\n", c.cfg.Server.Host)
		err = srv.ListenAndServe()
	}

	if err != nil {
		log.Printf("Listen failed: %s\n", err)
	}
}

func parseConfig(file string) config {
	f, err := os.Open(file)
	if err != nil {
		log.Fatalf("Error: %v", err)
	}
	defer f.Close()

	dec := yaml.NewDecoder(f)

	cfg := config{}
	err = dec.Decode(&cfg)
	if err != nil {
		log.Fatalf("Error: %v", err)
	}

	cfg.Workdir, _ = filepath.Abs(cfg.Workdir)
	cfg.Logdir, _ = filepath.Abs(cfg.Logdir)
	cfg.Server.BaseURL = strings.TrimSuffix(cfg.Server.BaseURL, "/")
	cfg.Repository.APIURL = strings.TrimSuffix(cfg.Repository.APIURL, "/")

	for i := range cfg.Queues {
		if cfg.Queues[i].PathMatch == "" {
			cfg.Queues[i].PathMatch = "^.*$"
		}

		_, err := regexp.Compile(cfg.Queues[i].PathMatch)
		if err != nil {
			log.Fatalf("Error: %v", err)
		}
	}

	return cfg
}

func main() {
	var cfgfile string

	flag.StringVar(&cfgfile, "config", "caronade.yaml", "Path to config file")
	flag.Parse()

	cfg := parseConfig(cfgfile)
	wg := sync.WaitGroup{}

	ctrl := controller{
		wg:     &wg,
		cfg:    &cfg,
		queues: make(map[string]*queue),
	}

	reg := regexp.MustCompile("[^a-zA-Z0-9]+")
	for i := range cfg.Queues {
		log.Printf("Adding queue %s\n", cfg.Queues[i].Name)
		cfg.Queues[i].Workdir = path.Join(cfg.Workdir, reg.ReplaceAllString(cfg.Queues[i].Name, ""))
		cfg.Queues[i].queue = make(chan *job, 10)
		ctrl.queues[cfg.Queues[i].Name] = &cfg.Queues[i]

		wg.Add(1)
		go ctrl.startWorker(&cfg.Queues[i])
	}

	wg.Add(1)
	go ctrl.startHTTPD()

	wg.Wait()
}
