package main

import (
	"context"
	"embed"
	"encoding/json"
	"flag"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path"
	"strconv"
	"sync"
	"time"
)

type Program struct {
	Name string
	Exec string
	Dir  string
}

type Control struct {
	stopc    chan bool
	errc     chan error
	finished bool
	status   error
}

type Instance struct {
	Name    string
	Prog    int
	Running bool
	Status  string
}

var (
	mut      sync.Mutex
	programs []Program
	running  []*Control
)

var (
	//go:embed templates
	templates embed.FS

	//go:embed static
	static embed.FS

	site *template.Template
)

var conf = flag.String("c", "./progs.json", "configuration file")
var addr = flag.String("l", ":8080", "listen at host:port")

func usage() {
	fmt.Fprintf(os.Stderr, `usage: wcon [-l host:port] [-c configuration]

Wcon is a simple task manager with a web interface. I use it to control simple
tasks running on my raspberries. Examples are music players, slide shows, simple
audio services etc. It uses a simple json configuration file:
[
  {
    "Name": "task display name",
    "Exec": "bash -c this",
    "Dir":  "working directory"
   },
   ...
]

The web server listens at host:port and presents a simple UI to start/stop tasks.
Usually i start it with systemd on boot.

Flags:
`)
	flag.PrintDefaults()
	os.Exit(2)
}

func main() {
	log.SetPrefix("")
	log.SetFlags(0)
	flag.Usage = usage
	flag.Parse()

	site = template.Must(template.ParseFS(templates, "templates/*.tmpl"))

	fin, err := os.Open(*conf)
	if err != nil {
		log.Fatal(err)
	}
	if err := json.NewDecoder(fin).Decode(&programs); err != nil {
		log.Fatal(err)
	}
	fin.Close()

	running = make([]*Control, len(programs))

	http.Handle("/static/", http.FileServer(http.FS(static)))
	http.Handle("/start/", http.StripPrefix("/start/", http.HandlerFunc(startHandler)))
	http.Handle("/stop/", http.StripPrefix("/stop/", http.HandlerFunc(stopHandler)))
	http.HandleFunc("/", indexHandler)

	server := &http.Server{
		Addr:              *addr,
		ReadHeaderTimeout: 5 * time.Second,
	}
	log.Fatal(server.ListenAndServe())
}

func ctlOf(prog int) (*Control, bool) {
	if prog < 0 || prog >= len(programs) {
		return nil, false
	}

	ctl := running[prog]
	return ctl, ctl != nil
}

func stop(prog int) {
	mut.Lock()
	defer mut.Unlock()

	if ctl, ok := ctlOf(prog); ok && !ctl.finished {
		ctl.stopc <- true
	}
}

func start(prog int) {
	mut.Lock()
	defer mut.Unlock()

	if ctl, ok := ctlOf(prog); !ok || ctl.finished {
		running[prog] = supervisor(programs[prog])
	}
}

func supervisor(p Program) *Control {
	ctl := &Control{
		stopc: make(chan bool),
		errc:  make(chan error),
	}

	go func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		go func() {
			cmd := exec.CommandContext(ctx, "bash", "-c", p.Exec)
			cmd.Dir = p.Dir
			if err := cmd.Start(); err != nil {
				ctl.errc <- err
			} else {
				ctl.errc <- cmd.Wait()
			}
		}()

		select {
		case <-ctl.stopc:
			// stopped by user, not an error
		case err := <-ctl.errc:
			ctl.status = err
		}
		ctl.finished = true
	}()

	return ctl
}

func startHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		fmt.Fprintf(w, "Use POST")
		return
	}

	dir, file := path.Split(r.URL.Path)
	if dir != "" {
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprintf(w, "Not Found")
		return
	}

	prog, err := strconv.Atoi(file)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprintf(w, "Not Found")
		return
	}

	start(prog)
	w.WriteHeader(http.StatusAccepted)
}

func stopHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		fmt.Fprintf(w, "Use POST")
		return
	}

	dir, file := path.Split(r.URL.Path)
	if dir != "" {
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprintf(w, "Not Found")
		return
	}

	prog, err := strconv.Atoi(file)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprintf(w, "Not Found")
		return
	}

	stop(prog)
	w.WriteHeader(http.StatusAccepted)
}

func indexHandler(w http.ResponseWriter, r *http.Request) {
	_ = site.ExecuteTemplate(w, "index.tmpl", instances())
}

func instances() []Instance {
	mut.Lock()
	defer mut.Unlock()

	var insts []Instance
	for id, prog := range programs {
		st := Instance{Name: prog.Name, Prog: id}
		if ctl, ok := ctlOf(id); ok {
			st.Running = !ctl.finished
			if ctl.finished && ctl.status != nil {
				st.Status = ctl.status.Error()
			}
		}
		insts = append(insts, st)
	}
	return insts
}
