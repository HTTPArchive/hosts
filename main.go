package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"
)

// {
// 	Host: "google.com"
// 	Requests: [
// 		{
// 			URL: "http://google.com",
// 			Status: 301
// 			Protocol: "HTTP/1.1"
// 			Location: "http://google.com/"
// 			TLSVersion: "1.2"
// 			TLSCipherSuite: "..."
// 		},
// 		{
//        ....
// 		}
// 	]
// }

type response struct {
	URL      string
	Status   int
	Protocol string
	Location string
	// TLSVersion     string
	// TLSCipherSuite uint16
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

// transport is an http.RoundTripper that keeps track of the in-flight
// request and implements hooks to report HTTP tracing events.
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
			URL:      req.URL.String(),
			Status:   resp.StatusCode,
			Protocol: resp.Proto,
			Location: resp.Header.Get("Location"),
			// TLSVersion: string(resp.TLS.Version),
			// TLSCipherSuite: resp.TLS.CipherSuite,
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

	res := &result{
		Host:        "igvita.com/",
		HTTPSOnly:   false,
		HTTPSuccess: true,
	}

	res.HTTPResponses, err = fetch("http://" + res.Host)
	if err != nil {
		res.HTTPSuccess = false
		log.Fatal(err)
	}

	finalHTTPResponse := res.HTTPResponses[len(res.HTTPResponses)-1]
	res.FinalLocation = finalHTTPResponse.URL

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
		res.FinalLocation = finalHTTPSResponse.URL
	}

	serialize, err := json.Marshal(res)
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println(string(serialize))
}
