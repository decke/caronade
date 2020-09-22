package server

import (
	"fmt"
	"io/ioutil"
	"strings"
	"time"
)

func (j *Job) StatusOverall() string {

	// status: waiting | building | failure | success
	allwaiting := true

	for _, b := range j.Build {
		if b.Status != "waiting" {
			allwaiting = false
		}
	}

	if allwaiting {
		return "waiting"
	}

	for _, b := range j.Build {
		if b.Status == "building" || b.Status == "waiting" {
			return "building"
		}
	}

	for _, b := range j.Build {
		if b.Status == "failure" {
			return b.Status
		}
	}

	return "success"
}

func (j *Job) Progress() int {
	jobs := 0.0
	done := 0.0

	for _, b := range j.Build {
		jobs += 1
		if b.Status == "failure" || b.Status == "success" {
			done += 1
		}
	}

	return int((done / jobs) * 100.0)
}

func (j *Job) StartDate() string {
	return j.Startdate.Format(time.RFC850)
}

func (j *Job) EndDate() string {
	return j.Enddate.Format(time.RFC850)
}

func (j *Job) TimeNow() string {
	return time.Now().Format(time.RFC850)
}

func (j *Job) JobRuntime() string {
	diff := j.Enddate.Sub(j.Startdate).Round(time.Second)

	if j.StatusOverall() == "waiting" || j.StatusOverall() == "building" {
		diff = time.Now().Sub(j.Startdate).Round(time.Second)
	}

	return fmt.Sprintf("%s", diff.String())
}

func (j *Job) ShortCommitID() string {
	return j.PushEvent.Commits[j.CommitIdx].CommitID[0:7]
}

func (j *Job) ShortCommitMessage() string {
	for _, line := range strings.Split(strings.TrimSuffix(j.PushEvent.Commits[j.CommitIdx].Message, "\n"), "\n") {
		return line
	}

	return ""
}

func (b *Build) Runtime() string {
	diff := b.Enddate.Sub(b.Startdate).Round(time.Second)

	if b.Status == "waiting" || b.Status == "building" {
		diff = time.Now().Sub(b.Startdate).Round(time.Second)
	}

	return fmt.Sprintf("%s", diff.String())
}

func (b *Build) LogfileContent() string {
	raw, err := ioutil.ReadFile(b.Logfile)
	if err != nil {
		return ""
	}

	return string(raw)
}
