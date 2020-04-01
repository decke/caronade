package server

import (
	"fmt"
	"io/ioutil"
	"time"
)

func (j *Job) StatusOverall() string {

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

	return fmt.Sprintf("%s", diff.String())
}

func (j *Job) JobIsToday() bool {
	start := j.Startdate
	now := time.Now()

	return (start.Year() == now.Year() && start.Month() == now.Month() && start.Day() == now.Day())
}

func (b *Build) Runtime() string {
	diff := b.Enddate.Sub(b.Startdate).Round(time.Second)

	return fmt.Sprintf("%s", diff.String())
}

func (b *Build) LogfileContent() string {
	raw, err := ioutil.ReadFile(b.Logfile)
	if err != nil {
		return ""
	}

	return string(raw)
}
