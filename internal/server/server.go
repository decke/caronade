package server

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path"
	"regexp"
	"sync"
	"time"

	texttemplate "text/template"
)

func (c *Controller) evalEnvVariable(j *Job, key string, val string) (string, string) {
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

func (c *Controller) startWorker(q *Queue) {
	defer c.wg.Done()

	for {
		var j *Job
		select {
		case j = <-q.Queue:
			b := j.Build[q.Name]
			b.Startdate = time.Now()

			log.Printf("ID %s: %s started on %s\n", j.ID, j.Port, q.Name)

			os.MkdirAll(path.Join(c.cfg.Logdir, j.ID), os.ModePerm)

			c.sendStatusUpdate(j, b)
			c.writeJsonExport(j)

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
			c.writeJsonExport(j)

		case <-time.After(time.Second * 1):
		}
	}
}

func StartServer(cfgfile string) {
	cfg := parseConfig(cfgfile)
	wg := sync.WaitGroup{}

	ctrl := Controller{
		wg:     &wg,
		cfg:    cfg,
		queues: make(map[string]*Queue),
	}

	reg := regexp.MustCompile("[^a-zA-Z0-9]+")
	for i := range cfg.Queues {
		log.Printf("Adding queue %s\n", cfg.Queues[i].Name)
		cfg.Queues[i].Workdir = path.Join(cfg.Workdir, reg.ReplaceAllString(cfg.Queues[i].Name, ""))
		cfg.Queues[i].Queue = make(chan *Job, 10)
		ctrl.queues[cfg.Queues[i].Name] = &cfg.Queues[i]

		wg.Add(1)
		go ctrl.startWorker(&cfg.Queues[i])
	}

	wg.Add(1)
	go ctrl.Serve()

	wg.Wait()
}
