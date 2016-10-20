package main

import (
	"bufio"
	"crypto/tls"
	"encoding/json"
	"flag"
	"fmt"
	"math"
	"net"
	"net/http"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"

	log "github.com/Sirupsen/logrus"
)

type response struct {
	RequestURL string
	Status     int
	Protocol   string
	Headers    http.Header
	TLS        *tls.ConnectionState
}

type result struct {
	Host          string
	FinalLocation string
	Error         string

	HTTPResponses  []response
	HTTPSResponses []response

	HTTPOk    bool
	HTTPSOk   bool
	HTTPSOnly bool
}

type transport struct {
	http.Transport
	Dial      net.Dialer
	Responses []response
}

var (
	wg sync.WaitGroup
)

func init() {
	// Log as JSON instead of the default ASCII formatter.
	// log.SetFormatter(&log.JSONFormatter{})
	log.SetFormatter(&log.TextFormatter{})

	// Output to stderr instead of stdout, could also be a file.
	log.SetOutput(os.Stderr)

	// Only log the warning severity or above.
	debug := os.Getenv("DEBUG")
	if debug == "true" {
		log.SetLevel(log.DebugLevel)
	} else {
		log.SetLevel(log.InfoLevel)
	}
}

func (t *transport) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := http.DefaultTransport.RoundTrip(req)
	if err != nil {
		return resp, err
	}
	defer resp.Body.Close()

	t.Responses = append(t.Responses,
		response{
			RequestURL: req.URL.String(),
			Status:     resp.StatusCode,
			Protocol:   resp.Proto,
			Headers:    resp.Header,
			TLS:        resp.TLS,
		})

	return resp, err
}

func fetch(url string) ([]response, error) {
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("User-Agent", "httparchive.org")

	t := &transport{}

	t.DisableKeepAlives = true
	t.TLSHandshakeTimeout = 3 * time.Second
	t.ExpectContinueTimeout = 1 * time.Second
	t.ResponseHeaderTimeout = 5 * time.Second

	client := &http.Client{
		Timeout:   6 * time.Second,
		Transport: t,
	}
	_, err := client.Do(req)

	return t.Responses, err
}

func collector(updateInterval time.Duration, out *os.File) chan<- *result {
	results := make(chan *result)
	ticker := time.NewTicker(updateInterval)

	start := time.Now()
	processed := 0
	go func() {
		for {
			select {
			case <-ticker.C:
				var m runtime.MemStats
				runtime.ReadMemStats(&m)
				elapsed := time.Since(start).Seconds()
				fmt.Printf("[Fetcher] processed %v, Rate: %.2f hosts/s. [Memory] Sys: %d, Alloc: %d, Objects: %d\n",
					processed, float64(processed)/elapsed, m.HeapSys, m.HeapAlloc, m.HeapObjects)
			}
		}
	}()

	go func() {
		defer wg.Done()

		for res := range results {
			log.WithFields(log.Fields{
				"host": res.Host,
			}).Debug("Writer recieved data")
			serialize, err := json.Marshal(res)
			if err != nil {
				fmt.Println(err)
				return
			}
			out.Write(serialize)
			out.Write([]byte("\n"))

			log.WithFields(log.Fields{
				"host":       res.Host,
				"https-only": res.HTTPSOnly,
				"location":   res.FinalLocation,
			}).Info("Writer flushed data")
			processed++
		}
	}()

	return results
}

func fetcher(in <-chan string, out chan<- *result) {
	defer wg.Done()

	for host := range in {
		res := &result{
			Host:      host,
			HTTPOk:    false,
			HTTPSOk:   false,
			HTTPSOnly: false,
		}
		log.WithFields(log.Fields{
			"host": res.Host,
		}).Debug("Starting HTTP check")

		var err error
		res.HTTPResponses, err = fetch("http://" + res.Host)
		if err != nil {
			log.Error(err)
			res.Error = err.Error()

			out <- res
			continue
		}

		finalHTTPResponse := res.HTTPResponses[len(res.HTTPResponses)-1]
		res.FinalLocation = finalHTTPResponse.RequestURL
		log.WithFields(log.Fields{
			"host":   res.Host,
			"status": finalHTTPResponse.Status,
		}).Debug("Processed HTTP host")

		if finalHTTPResponse.Status == 200 {
			res.HTTPOk = true
		}

		if strings.HasPrefix(res.FinalLocation, "https://") {
			log.WithFields(log.Fields{
				"host": res.Host,
			}).Debug("Skipping HTTPS check; HTTP -> HTTPS")

			res.HTTPSOk = true
			res.HTTPSOnly = true
		} else {
			log.WithFields(log.Fields{
				"host": res.Host,
			}).Debug("Starting HTTPS check")

			res.HTTPSResponses, err = fetch("https://" + res.Host)
			if err != nil {
				log.Error(err)
				res.Error = err.Error()

				out <- res
				continue
			}

			finalHTTPSResponse := res.HTTPSResponses[len(res.HTTPSResponses)-1]
			res.FinalLocation = finalHTTPSResponse.RequestURL

			if finalHTTPSResponse.Status == 200 {
				res.HTTPSOk = true
			}

			log.WithFields(log.Fields{
				"host":   res.Host,
				"status": finalHTTPSResponse.Status,
			}).Debug("Processed HTTPS host.")
		}

		log.WithFields(log.Fields{
			"host":       res.Host,
			"https-only": res.HTTPSOnly,
		}).Debug("Finished processing HTTP + HTTPS")

		out <- res
	}
}

func main() {
	// Parse command line flags
	workers := flag.Int("workers", 10, "number of parallel HTTP fetches")
	output := flag.String("output", "results.json", "output file")
	flag.Parse()

	out, err := os.Create(*output)
	if err != nil {
		log.Fatal(err)
	}
	defer out.Close()

	resultQueue := collector(5*time.Second, out)
	workQueue := make(chan string, int(math.Max(float64(*workers), float64(1))))

	for i := 0; i < int(math.Max(float64(*workers/2), float64(1))); i++ {
		wg.Add(1)
		go fetcher(workQueue, resultQueue)
	}

	// Read TLD's from STDIN and queue for processing
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		workQueue <- scanner.Text()
	}

	// All the URLS have been queued, close the channel and
	// wait for the fetcher routines to drain the channel
	close(workQueue)
	wg.Wait()

	// Wait for the collector to signal that it has finished
	// writing out all the results
	close(resultQueue)
	wg.Add(1)
	wg.Wait()
}
