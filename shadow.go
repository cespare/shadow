package main

import (
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/cespare/hutil/apachelog"
)

type shadow struct {
	graphiteURL string
	client      *http.Client
}

// A status is an HTTP status code plus a reason if the code is not 200.
type status struct {
	code    int
	message string
}

func (s *status) setFromResponse(resp *http.Response) {
	s.code = resp.StatusCode
	if s.code == 200 {
		s.message = ""
		return
	}
	b, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		s.message = fmt.Sprintf("Could not read server response: %s", err)
		return
	}
	s.message = string(b)
}

func (s *status) write(w http.ResponseWriter) {
	w.WriteHeader(s.code)
	if s.code == 200 {
		io.WriteString(w, "OK\n")
		return
	}
	fmt.Fprintf(w, "NOT OK (%d): %s\n", s.code, s.message)
}

func main() {
	log.SetFlags(0)
	addr := flag.String("addr", ":8080", "Listen addr")
	graphiteURL := flag.String("graphite", "", "Graphite root URL")
	graphiteTimeout := flag.Duration("timeout", 30*time.Second, "Graphite request timeout")
	flag.Parse()

	if *graphiteURL == "" {
		log.Fatal("-graphite must be given")
	}
	if !strings.Contains(*graphiteURL, "://") {
		*graphiteURL = "http://" + *graphiteURL
	}
	*graphiteURL = strings.TrimSuffix(*graphiteURL, "/")

	s := &shadow{
		client: &http.Client{
			Transport: &http.Transport{
				// Dial with 5 second timeout (including name resolution).
				DialContext:         (&net.Dialer{Timeout: 5 * time.Second}).DialContext,
				MaxIdleConnsPerHost: 10,
			},
			Timeout: *graphiteTimeout,
		},
		graphiteURL: *graphiteURL,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/check", s.handleGraphiteCheck)
	mux.Handle("/", http.FileServer(http.Dir("static")))

	server := &http.Server{
		Addr:    *addr,
		Handler: apachelog.NewDefaultHandler(mux),
	}
	log.Println("Now listening on", *addr)
	log.Fatal(server.ListenAndServe())
}
