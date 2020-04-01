package server

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"net/smtp"
	"os"
	"path"
	"regexp"
	"strings"
	texttemplate "text/template"
)

func getAffectedPorts(data GitPushEventData, commit int) map[string]int {
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

func (c *Controller) matchQueues(data GitPushEventData, commit int) []Queue {
	queues := make([]Queue, 0)

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

func (c *Controller) renderBuildTemplate(j *Job) {
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

func (c *Controller) writeJsonExport(j *Job) {
	file, _ := json.MarshalIndent(j, "", " ")
	_ = ioutil.WriteFile(path.Join(c.cfg.Logdir, j.ID, "data.json"), file, 0644)
}

func (c *Controller) renderEmailTemplate(j *Job) string {
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

func (c *Controller) sendStatusUpdate(j *Job, b *Build) error {
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

