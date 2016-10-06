package main

import (
	"bufio"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
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

	HTTPResponses []response
	HTTPSuccess   bool

	HTTPSResponses []response
	HTTPSSuccess   bool
	HTTPSOnly      bool
}

type transport struct {
	http.Transport
	Responses []response
}

var (
	wg       sync.WaitGroup
	errorLog *log.Logger
)

func (t *transport) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := http.DefaultTransport.RoundTrip(req)
	if err != nil {
		return resp, err
	}

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
	req.Header.Set("User-Agent", "YourUserAgentString")

	t := &transport{}
	t.DisableKeepAlives = true
	t.TLSHandshakeTimeout = 5 * time.Second
	t.ExpectContinueTimeout = 1 * time.Second
	t.ResponseHeaderTimeout = 10 * time.Second

	client := &http.Client{Transport: t}
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
				elapsed := time.Since(start).Seconds()
				fmt.Printf("Processed: %v, Rate: %.2f hosts/s\n",
					processed, float64(processed)/elapsed)
			}
		}
	}()

	go func() {
		defer wg.Done()

		for res := range results {
			serialize, err := json.Marshal(res)
			if err != nil {
				fmt.Println(err)
				return
			}
			out.Write(serialize)
			out.Write([]byte("\n"))

			processed++
		}
	}()

	return results
}

func fetcher(in <-chan string, out chan<- *result) {
	defer wg.Done()

	for host := range in {
		res := &result{
			Host:        host + "/",
			HTTPSOnly:   false,
			HTTPSuccess: true,
		}

		var err error
		res.HTTPResponses, err = fetch("http://" + res.Host)
		if err != nil {
			errorLog.Println(err)
			res.Error = err.Error()

			out <- res
			return
		}

		finalHTTPResponse := res.HTTPResponses[len(res.HTTPResponses)-1]
		res.FinalLocation = finalHTTPResponse.RequestURL

		if strings.HasPrefix(res.FinalLocation, "https://") {
			res.HTTPSOnly = true
		} else {
			res.HTTPSResponses, err = fetch("https://" + res.Host)
			if err != nil {
				errorLog.Println(err)
				res.Error = err.Error()

				out <- res
				return
			}

			finalHTTPSResponse := res.HTTPSResponses[len(res.HTTPSResponses)-1]
			res.FinalLocation = finalHTTPSResponse.RequestURL
		}

		out <- res
	}
}

func main() {
	errorLog = log.New(os.Stderr, "ERROR: ", log.Ldate)

	out, err := os.Create("results.json")
	if err != nil {
		log.Fatal(err)
	}
	defer out.Close()

	workQueue := make(chan string, 100)
	resultQueue := collector(1*time.Second, out)

	for i := 0; i < 5; i++ {
		wg.Add(1)
		go fetcher(workQueue, resultQueue)
	}

	// Read TLD's from STDIN and queue for processing
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		host := scanner.Text()
		workQueue <- host
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
