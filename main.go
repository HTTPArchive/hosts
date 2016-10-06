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

func main() {
	var err error

	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		host := scanner.Text()
		fmt.Printf("Processing %v\n", host)

		res := &result{
			Host:        host + "/",
			HTTPSOnly:   false,
			HTTPSuccess: true,
		}

		res.HTTPResponses, err = fetch("http://" + res.Host)
		if err != nil {
			res.HTTPSuccess = false
			log.Fatal(err)
		}

		finalHTTPResponse := res.HTTPResponses[len(res.HTTPResponses)-1]
		res.FinalLocation = finalHTTPResponse.RequestURL

		if strings.HasPrefix(res.FinalLocation, "https://") {
			res.HTTPSSuccess = true
			res.HTTPSOnly = true
		} else {
			res.HTTPSResponses, err = fetch("https://" + res.Host)
			if err != nil {
				res.HTTPSSuccess = false
				log.Fatal(err)
			}

			finalHTTPSResponse := res.HTTPSResponses[len(res.HTTPSResponses)-1]
			res.FinalLocation = finalHTTPSResponse.RequestURL
		}

		serialize, err := json.Marshal(res)
		if err != nil {
			fmt.Println(err)
			return
		}
		fmt.Println(string(serialize))

	}
}
