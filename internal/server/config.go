package server

import (
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v2"
)

func parseConfig(file string) *Config {
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

	return &cfg
}
