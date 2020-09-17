package server

import (
	"sync"
	"time"
)

type Controller struct {
	wg     *sync.WaitGroup
	cfg    *Config
	queues map[string]*Queue
}

type Config struct {
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
	Queues        []Queue
	DefaultQueues []string `yaml:"default_queues"`
}

type Queue struct {
	Name        string
	Recipe      string
	Environment map[string]string
	Workdir     string
	PathMatch   string
	Queue       chan *Job
}

type Job struct {
	ID        string
	Port      string
	Startdate time.Time
	Enddate   time.Time
	Build     map[string]*Build
	PushEvent GitPushEventData
	CommitIdx int
	BaseURL   string
	Nonce     string
}

type Build struct {
	ID        string
	Queue     string
	Status    string
	Logfile   string
	Startdate time.Time
	Enddate   time.Time
}

type Jobs struct {
	Filter string
	Jobs   []Job
	Nonce  string
}

type GitPushEventData struct {
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
