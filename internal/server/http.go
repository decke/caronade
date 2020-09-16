package server

import (
	"fmt"
	"html/template"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"path"
	"sort"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/unrolled/secure"
)

type Template struct {
	templates *template.Template
}

func (t *Template) Render(w io.Writer, name string, data interface{}, c echo.Context) error {
	return t.templates.ExecuteTemplate(w, name, data)
}

func (c *Controller) handleJobListing(ctx echo.Context) error {
	files, err := ioutil.ReadDir(c.cfg.Logdir)
	if err != nil {
		return ctx.String(http.StatusInternalServerError, "Internal Error (dirlisting failed)")
	}

	sort.Slice(files, func(i, j int) bool {
		return files[i].ModTime().Unix() > files[j].ModTime().Unix()
	})

	jobs := Jobs{
		Filter: ctx.FormValue("when"),
		Jobs:   make([]Job, 0),
		Nonce:  secure.CSPNonce(ctx.Request().Context()),
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

	return ctx.Render(http.StatusOK, "joblisting.html", &jobs)
}

func (c *Controller) handleWebhook(ctx echo.Context) error {
	if ctx.Request().Header.Get("X-GitHub-Event") == "ping" {
		return ctx.String(http.StatusOK, "pong")
	}

	if ctx.Request().Header.Get("X-GitHub-Event") != "push" {
		return ctx.String(http.StatusBadRequest, "Invalid webhook")
	}

	data := new(GitPushEventData)
	if err := ctx.Bind(data); err != nil {
		return ctx.String(http.StatusBadRequest, "Failed to parse webhook data")
	}

	output := ""

	for commit := range data.Commits {
		for port := range getAffectedPorts(*data, commit) {
			job := Job{
				ID:        time.Now().Format("20060102-15:04:05.00000"),
				Port:      port,
				Startdate: time.Now(),
				Build:     make(map[string]*Build),
				PushEvent: *data,
				CommitIdx: commit,
				BaseURL:   "",
			}

			job.BaseURL = fmt.Sprintf("%s/%s/%s/", c.cfg.Server.BaseURL, "builds", job.ID)

			cnt := 0
			for _, q := range c.matchQueues(*data, job.CommitIdx) {
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
			output = output + fmt.Sprintf("ID %s: %d jobs for port %s\n", job.ID, cnt, job.Port)
		}
	}

	return ctx.String(http.StatusOK, output)
}

func (c *Controller) Serve() {
	e := echo.New()
	e.HideBanner = true

	e.Use(middleware.Logger())
	e.Use(middleware.Gzip())

	secureMiddleware := secure.New(secure.Options{
		FrameDeny:             true,
		ContentSecurityPolicy: "default-src 'none'; style-src 'self'; img-src 'self'; font-src 'self'; base-uri 'none'; frame-ancestors 'none'; form-action 'self'; script-src 'strict-dynamic' $NONCE 'unsafe-inline' http: https:;",
	})

	e.Use(echo.WrapMiddleware(secureMiddleware.Handler))

	e.Static("/static", c.cfg.Staticdir)
	e.Static("/builds", c.cfg.Logdir)

	t := &Template{
		templates: template.Must(template.ParseGlob(path.Join(c.cfg.Tmpldir, "*.html"))),
	}

	e.Renderer = t

	e.GET("/", c.handleJobListing)

	if c.cfg.Webhook.Secret != "" {
		e.POST("/", c.handleWebhook, HmacAuth(c.cfg.Webhook.Secret))
	} else {
		e.POST("/", c.handleWebhook)
	}

	if c.cfg.Server.TLScert != "" && c.cfg.Server.TLSkey != "" {
		e.Logger.Fatal(e.StartTLS(c.cfg.Server.Host, c.cfg.Server.TLScert, c.cfg.Server.TLSkey))
	} else {
		e.Logger.Fatal(e.Start(c.cfg.Server.Host))
	}
}
