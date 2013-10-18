package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/BurntSushi/toml"
	apachelog "github.com/cespare/go-apachelog"
)

var (
	client = &http.Client{
		Transport: &http.Transport{
			MaxIdleConnsPerHost:   10,
			ResponseHeaderTimeout: 10 * time.Second,
		},
	}
	conf     *Conf
	confFile = flag.String("conf", "conf.toml", "Which config file to use")
)

func init() {
	flag.Parse()
	conf = &Conf{}
	_, err := toml.DecodeFile(*confFile, conf)
	if err != nil {
		log.Fatal(err)
	}
	if !(strings.HasPrefix(conf.GraphiteAddr, "http://") || strings.HasPrefix(conf.GraphiteAddr, "https://")) {
		conf.GraphiteAddr = "http://" + conf.GraphiteAddr
	}
	conf.GraphiteAddr = strings.TrimSuffix(conf.GraphiteAddr, "/")
}

func GraphiteURL(path string) string {
	return fmt.Sprintf("%s/%s", conf.GraphiteAddr, strings.TrimPrefix(path, "/"))
}

type Conf struct {
	ListenAddr   string `toml:"listen_addr"`
	GraphiteAddr string `toml:"graphite_addr"`
}

// A Status is an HTTP status and a reason if it's not http.StatusOK.
type Status struct {
	Code    int
	Message string
}

func (s *Status) SetFromResponse(resp *http.Response) {
	s.Code = resp.StatusCode
	if s.Code == http.StatusOK {
		s.Message = ""
		return
	}
	b, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		s.Message = fmt.Sprintf("Could not read server response: %s", err)
		return
	}
	s.Message = string(b)
}

func (s *Status) WriteHTTPResponse(w http.ResponseWriter) {
	w.WriteHeader(s.Code)
	w.Write([]byte(s.String()))
}

func (s *Status) String() string {
	if s.Code == http.StatusOK {
		return "OK"
	}
	return fmt.Sprintf("NOT OK (%d): %s", s.Code, s.Message)
}

// SelfHealthChecker is our own health. We're assumed to be OK if we're running and if the Graphite we depend
// on is running.
type SelfHealthChecker struct {
	sync.Mutex
	*Status
	Frequency time.Duration
}

func (c *SelfHealthChecker) do() {
	log.Println("Running graphite health check")
	resp, err := client.Get(GraphiteURL("/")) // TODO: Is there a better health check route?
	c.Lock()
	defer c.Unlock()
	if err != nil {
		log.Println("Error contacting graphite:", err)
		c.Code = http.StatusBadGateway
		c.Message = fmt.Sprint("Error contacting graphite server:", err)
		return
	}
	log.Println("Graphite OK")
	c.SetFromResponse(resp)
	resp.Body.Close()
}

func (c *SelfHealthChecker) Run() {
	ticker := time.NewTicker(c.Frequency)
	c.do()
	for _ = range ticker.C {
		c.do()
	}
}

func (c *SelfHealthChecker) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	c.Lock()
	c.WriteHTTPResponse(w)
	c.Unlock()
}

func main() {
	selfChecker := &SelfHealthChecker{
		Status: &Status{
			Code:    http.StatusNotFound,
			Message: "Health check hasn't been run against Graphite yet",
		},
		Frequency: 30 * time.Second,
	}
	go selfChecker.Run()

	mux := http.NewServeMux()
	mux.Handle("/healthz", selfChecker)
	mux.HandleFunc("/check", HandleGraphiteChecks)

	server := &http.Server{
		Addr:    conf.ListenAddr,
		Handler: apachelog.NewHandler(mux, os.Stderr),
	}
	log.Println("Now listening on", conf.ListenAddr)
	log.Fatal(server.ListenAndServe())
}
