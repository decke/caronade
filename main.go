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
	"net"
	"net/http"
	"net/smtp"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	texttemplate "text/template"
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
	Notification struct {
		StatusAPI struct {
			Token string
		}
		Email struct {
			SmtpHost string
			SmtpUser string
			SmtpPass string
			From     string
		}
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
	Port      string
	Startdate time.Time
	Enddate   time.Time
	Build     map[string]*build
	PushEvent gitPushEventData
	CommitIdx int
	BaseURL   string
}

type build struct {
	ID        string
	Queue     string
	Status    string
	Logfile   string
	Startdate time.Time
	Enddate   time.Time
}

type jobs struct {
	Filter string
	Jobs   []job
}

type gitPushEventData struct {
	Repository struct {
		Name      string `json:"name"`
		FullName  string `json:"full_name"`
		HTMLURL   string `json:"html_url"`
		StatusURL string `json:"statuses_url"`
		CloneURL  string `json:"clone_url"`
	} `json:"repository"`
	Commits []struct {
		CommitID string `json:"id"`
		Message  string `json:"message"`
		URL      string `json:"url"`
		Author   struct {
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

func getAffectedPorts(data gitPushEventData, commit int) map[string]int {
	ports := make(map[string]int, 0)

	re := regexp.MustCompile(`/`)

	for _, file := range data.Commits[commit].Added {
		parts := re.Split(file, -1)

		if len(parts) > 2 {
			port := fmt.Sprintf("%s/%s", parts[0], parts[1])
			ports[port] = 0
		}
	}

	for _, file := range data.Commits[commit].Modified {
		parts := re.Split(file, -1)

		if len(parts) > 2 {
			port := fmt.Sprintf("%s/%s", parts[0], parts[1])
			ports[port] = 0
		}
	}

	return ports
}

func (c *controller) matchQueues(data gitPushEventData, commit int) []queue {
	queues := make([]queue, 0)

NEXTQUEUE:
	for i := range c.cfg.Queues {
		re := regexp.MustCompile(c.cfg.Queues[i].PathMatch)

		// Queue name match against PathMatch config
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

		// Queue name from commit message tags (CI: yes/no/true/false)
		lines := strings.Split(data.Commits[commit].Message, "\n")
		for _, line := range lines {
			line = strings.ToLower(line)
			if strings.HasPrefix(line, "ci:") {
				if strings.Contains(line, "no") || strings.Contains(line, "false") {
					continue NEXTQUEUE
				}
				if strings.Contains(line, "yes") || strings.Contains(line, "true") {
					queues = append(queues, c.cfg.Queues[i])
					continue NEXTQUEUE
				}
			}
		}

		// Queue name from DefaultQueues config
		for _, q := range c.cfg.DefaultQueues {
			if q == c.cfg.Queues[i].Name {
				queues = append(queues, c.cfg.Queues[i])
			}
		}
	}

	return queues
}

func (j *job) StatusOverall() string {

	// status: pending | failure | success

	for _, b := range j.Build {
		if b.Status == "pending" {
			return b.Status
		}
	}

	for _, b := range j.Build {
		if b.Status == "failure" {
			return b.Status
		}
	}

	return "success"
}

func (j *job) StartDate() string {
	return j.Startdate.Format(time.RFC850)
}

func (j *job) EndDate() string {
	return j.Enddate.Format(time.RFC850)
}

func (j *job) TimeNow() string {
	return time.Now().Format(time.RFC850)
}

func (j *job) JobRuntime() string {
	diff := j.Enddate.Sub(j.Startdate).Round(time.Second)

	return fmt.Sprintf("%s", diff.String())
}

func (j *job) JobIsToday() bool {
	start := j.Startdate
	now := time.Now()

	return (start.Year() == now.Year() && start.Month() == now.Month() && start.Day() == now.Day())
}

func (b *build) Runtime() string {
	diff := b.Enddate.Sub(b.Startdate).Round(time.Second)

	return fmt.Sprintf("%s", diff.String())
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

func (c *controller) renderEmailTemplate(j *job) string {
	tmpl, err := texttemplate.ParseFiles(path.Join(c.cfg.Tmpldir, "email.txt"))
	if err != nil {
		log.Printf("Failed parsing template: %v", err)
		return ""
	}

	var out bytes.Buffer
	err = tmpl.Execute(&out, &j)
	if err != nil {
		log.Printf("Failed executing template: %v", err)
		return ""
	}

	return out.String()
}

func (c *controller) evalEnvVariable(j *job, key string, val string) (string, string) {
	tmpl, err := texttemplate.New(key).Parse(val)
	if err != nil {
		log.Printf("Failed parsing env var %s=%s: %v", key, val, err)
		return key, ""
	}

	var out bytes.Buffer
	err = tmpl.Execute(&out, &j)
	if err != nil {
		log.Printf("Failed executing env var %s: %v", key, err)
		return key, ""
	}

	return key, out.String()
}

func (c *controller) sendStatusUpdate(j *job, b *build) error {
	target := ""

	if b.Status != "pending" {
		target = j.BaseURL
	}

	if c.cfg.Notification.StatusAPI.Token != "" {
		url := strings.Replace(j.PushEvent.Repository.StatusURL, "{sha}", j.PushEvent.Commits[j.CommitIdx].CommitID, -1)
		jsonValue, _ := json.Marshal(map[string]string{
			"state":      b.Status,
			"target_url": target,
			"context":    j.Port + " on " + b.Queue,
		})

		req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonValue))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "token "+c.cfg.Notification.StatusAPI.Token)

		client := http.Client{}
		resp, err := client.Do(req)

		if err != nil {
			log.Printf("StatusAPI request to %s failed: %s\n", url, err)
		}

		resp.Body.Close()
	}

	if c.cfg.Notification.Email.SmtpHost != "" && j.StatusOverall() != "pending" {
		data := c.renderEmailTemplate(j)
		if data != "" {
			var auth smtp.Auth
			host, _, _ := net.SplitHostPort(c.cfg.Notification.Email.SmtpHost)

			if c.cfg.Notification.Email.SmtpUser != "" && c.cfg.Notification.Email.SmtpPass != "" {
				auth = smtp.PlainAuth("", c.cfg.Notification.Email.SmtpUser, c.cfg.Notification.Email.SmtpPass, host)
			}

			err := smtp.SendMail(
				c.cfg.Notification.Email.SmtpHost,
				auth,
				c.cfg.Notification.Email.From,
				[]string{j.PushEvent.Commits[j.CommitIdx].Author.EMail},
				[]byte(data),
			)
			if err != nil {
				log.Printf("EMail delivery failed: %v\n", err)
			}
		}
	}

	return nil
}

func (c *controller) startWorker(q *queue) {
	defer c.wg.Done()

	for {
		var j *job
		select {
		case j = <-q.queue:
			b := j.Build[q.Name]
			b.Startdate = time.Now()

			log.Printf("ID %s: %s started on %s\n", j.ID, j.Port, q.Name)

			os.MkdirAll(path.Join(c.cfg.Logdir, j.ID), os.ModePerm)

			c.sendStatusUpdate(j, b)
			c.renderBuildTemplate(j)

			env := os.Environ()
			for k, v := range q.Environment {
				key, val := c.evalEnvVariable(j, k, v)
				env = append(env, fmt.Sprintf("%s=%s", key, val))
			}

			os.MkdirAll(q.Workdir, os.ModePerm)

			cmd := exec.Cmd{
				Dir:  q.Workdir,
				Env:  env,
				Path: "/usr/bin/make",
				Args: []string{
					"make",
					"-C", q.Workdir,
					"-f", fmt.Sprintf("%s/%s.mk", c.cfg.Workdir, q.Recipe),
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
			ioutil.WriteFile(b.Logfile, output, 0644)

			log.Printf("ID %s: %s on %s finished %s\n", j.ID, j.Port, q.Name, b.Status)
			c.sendStatusUpdate(j, b)
			c.renderBuildTemplate(j)

		case <-time.After(time.Second * 1):
		}
	}
}

func (c *controller) handleJobListing(w http.ResponseWriter, r *http.Request) {
	files, err := ioutil.ReadDir(c.cfg.Logdir)
	if err != nil {
		http.Error(w, "Internal Error (dirlisting failed)", http.StatusInternalServerError)
		return
	}

	sort.Slice(files, func(i, j int) bool {
		return files[i].ModTime().Unix() > files[j].ModTime().Unix()
	})

	jobs := jobs{
		Filter: r.URL.Query().Get("when"),
		Jobs:   make([]job, 0),
	}

	if jobs.Filter == "" || jobs.Filter == "today" {
		jobs.Filter = "today"
	} else {
		jobs.Filter = "all"
	}

	for _, f := range files {
		t, _ := time.Parse("20060102-15:04:05.00000", f.Name())

		job := job{
			ID:        f.Name(),
			Port:      "",
			Startdate: t,
			Enddate:   f.ModTime(),
			BaseURL:   "",
		}

		job.BaseURL = fmt.Sprintf("%s/%s/%s/", c.cfg.Server.BaseURL, "builds", job.ID)

		if jobs.Filter == "today" {
			if job.JobIsToday() {
				jobs.Jobs = append(jobs.Jobs, job)
			}
		} else {
			jobs.Jobs = append(jobs.Jobs, job)
		}
	}

	tmpl, err := template.ParseFiles(path.Join(c.cfg.Tmpldir, "joblisting.html"))
	if err != nil {
		http.Error(w, "Internal Error (failed parsing template)", http.StatusInternalServerError)
		return
	}

	err = tmpl.Execute(w, &jobs)
	if err != nil {
		http.Error(w, "Internal Error (failed executing template)", http.StatusInternalServerError)
		return
	}
}

func (c *controller) handleWebhook(w http.ResponseWriter, r *http.Request) {
	payload, err := ioutil.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Internal Error", http.StatusInternalServerError)
		return
	}

	if c.cfg.Webhook.Secret != "" {
		if calcSignature(&payload, c.cfg.Webhook.Secret) != r.Header.Get("X-Hub-Signature") {
			http.Error(w, "Invalid secret", http.StatusBadRequest)
			return
		}
	}

	if r.Header.Get("X-GitHub-Event") == "ping" {
		fmt.Fprint(w, "pong")
		return
	}

	if r.Header.Get("X-GitHub-Event") != "push" {
		http.Error(w, "Invalid webhook", http.StatusBadRequest)
		return
	}

	data := gitPushEventData{}
	if err = json.Unmarshal(payload, &data); err != nil {
		http.Error(w, "Failed to parse webhook data", http.StatusBadRequest)
		return
	}

	for commit := range data.Commits {
		for port := range getAffectedPorts(data, commit) {
			job := job{
				ID:        time.Now().Format("20060102-15:04:05.00000"),
				Port:      port,
				Startdate: time.Now(),
				Build:     make(map[string]*build),
				PushEvent: data,
				CommitIdx: commit,
				BaseURL:   "",
			}

			job.BaseURL = fmt.Sprintf("%s/%s/%s/", c.cfg.Server.BaseURL, "builds", job.ID)

			cnt := 0
			for _, q := range c.matchQueues(data, job.CommitIdx) {
				b := build{
					ID:     fmt.Sprintf("%03d", cnt+1),
					Queue:  q.Name,
					Status: "pending",
				}
				job.Build[q.Name] = &b

				select {
				case q.queue <- &job:
					cnt++
					log.Printf("ID %s: job for %s queued on %s (pos %d)\n", job.ID, job.Port, q.Name, len(q.queue))
				default:
					log.Printf("ID %s: Queue limit reached on queue %s\n", job.ID, q.Name)
				}
			}
			fmt.Fprintf(w, "ID %s: %d jobs for port %s\n", job.ID, cnt, job.Port)
		}
	}
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
			c.handleJobListing(w, r)
		} else {
			c.handleWebhook(w, r)
		}
	})

	var err error
	if c.cfg.Server.TLScert != "" && c.cfg.Server.TLSkey != "" {
		// generated 2020-03-29, Mozilla Guideline v5.4, Golang 1.13.6, intermediate configuration
		cfg := &tls.Config{
			MinVersion:               tls.VersionTLS12,
			PreferServerCipherSuites: false,
			CipherSuites: []uint16{
				tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
				tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
				tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
				tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
				tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305,
				tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305,
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

	for i := range cfg.Queues {
		if cfg.Queues[i].PathMatch == "" {
			cfg.Queues[i].PathMatch = "^$"
		}
		_, err := regexp.Compile(cfg.Queues[i].PathMatch)
		if err != nil {
			log.Fatalf("Error: %v", err)
		}

		if cfg.Queues[i].Environment == nil {
			cfg.Queues[i].Environment = map[string]string{}
		}

		_, ok := cfg.Queues[i].Environment["JOB_ID"]
		if !ok {
			cfg.Queues[i].Environment["JOB_ID"] = "{{.ID}}"
		}

		_, ok = cfg.Queues[i].Environment["JOB_PORT"]
		if !ok {
			cfg.Queues[i].Environment["JOB_PORT"] = "{{.Port}}"
		}

		_, ok = cfg.Queues[i].Environment["COMMIT_ID"]
		if !ok {
			cfg.Queues[i].Environment["COMMIT_ID"] = "{{(index .PushEvent.Commits .CommitIdx).CommitID}}"
		}

		_, ok = cfg.Queues[i].Environment["REPO_URL"]
		if !ok {
			cfg.Queues[i].Environment["REPO_URL"] = "{{.PushEvent.Repository.CloneURL}}"
		}

		_, ok = cfg.Queues[i].Environment["AUTHOR"]
		if !ok {
			cfg.Queues[i].Environment["AUTHOR"] = "{{(index .PushEvent.Commits .CommitIdx).Author.Username}}"
		}

		_, ok = cfg.Queues[i].Environment["AUTHOR_EMAIL"]
		if !ok {
			cfg.Queues[i].Environment["AUTHOR_EMAIL"] = "{{(index .PushEvent.Commits .CommitIdx).Author.EMail}}"
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
