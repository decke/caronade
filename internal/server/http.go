package server

import (
	"crypto/hmac"
	"crypto/sha1"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"html/template"
	"io/ioutil"
	"log"
	"net/http"
	"path"
	"sort"
	"time"

	"github.com/NYTimes/gziphandler"
)

func calcSignature(payload *[]byte, secret string) string {
	mac := hmac.New(sha1.New, []byte(secret))
	mac.Write(*payload)

	return fmt.Sprintf("sha1=%x", mac.Sum(nil))
}

func (c *Controller) handleJobListing(w http.ResponseWriter, r *http.Request) {
	files, err := ioutil.ReadDir(c.cfg.Logdir)
	if err != nil {
		http.Error(w, "Internal Error (dirlisting failed)", http.StatusInternalServerError)
		return
	}

	sort.Slice(files, func(i, j int) bool {
		return files[i].ModTime().Unix() > files[j].ModTime().Unix()
	})

	jobs := Jobs{
		Filter: r.URL.Query().Get("when"),
		Jobs:   make([]Job, 0),
	}

	if jobs.Filter == "" || jobs.Filter == "today" {
		jobs.Filter = "today"
	} else {
		jobs.Filter = "all"
	}

	for _, f := range files {
		t, _ := time.Parse("20060102-15:04:05.00000", f.Name())

		job := Job{
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

func (c *Controller) handleWebhook(w http.ResponseWriter, r *http.Request) {
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

	data := GitPushEventData{}
	if err = json.Unmarshal(payload, &data); err != nil {
		http.Error(w, "Failed to parse webhook data", http.StatusBadRequest)
		return
	}

	for commit := range data.Commits {
		for port := range getAffectedPorts(data, commit) {
			job := Job{
				ID:        time.Now().Format("20060102-15:04:05.00000"),
				Port:      port,
				Startdate: time.Now(),
				Build:     make(map[string]*Build),
				PushEvent: data,
				CommitIdx: commit,
				BaseURL:   "",
			}

			job.BaseURL = fmt.Sprintf("%s/%s/%s/", c.cfg.Server.BaseURL, "builds", job.ID)

			cnt := 0
			for _, q := range c.matchQueues(data, job.CommitIdx) {
				b := Build{
					ID:     fmt.Sprintf("%03d", cnt+1),
					Queue:  q.Name,
					Status: "pending",
				}
				job.Build[q.Name] = &b

				select {
				case q.Queue <- &job:
					cnt++
					log.Printf("ID %s: job for %s queued on %s (pos %d)\n", job.ID, job.Port, q.Name, len(q.Queue))
				default:
					log.Printf("ID %s: Queue limit reached on queue %s\n", job.ID, q.Name)
				}
			}
			fmt.Fprintf(w, "ID %s: %d jobs for port %s\n", job.ID, cnt, job.Port)
		}
	}
}

func (c *Controller) startHTTPD() {
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
