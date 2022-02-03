package erclient

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"time"

	"github.com/luqmanMohammed/eventsrunner-k8s-sensor/sensor/eventqueue"
)

type EventsRunnerClientOpts struct {
	EventsRunnerBaseURL string
	CaCertPath          string
	ClientKeyPath       string
	ClientCertPath      string
	RequestTimeout      time.Duration
	JWTToken            string
}

type EventsRunnerClient struct {
	eventsRunnerBaseURL string
	httpClient          *http.Client
	headers             map[string]string
}

func NewMTLSClient(erClientOpts EventsRunnerClientOpts) (*EventsRunnerClient, error) {
	caCert, err := ioutil.ReadFile(erClientOpts.CaCertPath)
	if err != nil {
		return nil, err
	}
	caCertPool := x509.NewCertPool()
	if ok := caCertPool.AppendCertsFromPEM(caCert); !ok {
		return nil, errors.New("failed to setup tls config")
	}
	clientCert, err := tls.LoadX509KeyPair(erClientOpts.ClientCertPath, erClientOpts.ClientKeyPath)
	if err != nil {
		return nil, err
	}
	httpClient := &http.Client{
		Timeout: erClientOpts.RequestTimeout,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				RootCAs:      caCertPool,
				Certificates: []tls.Certificate{clientCert},
			},
		},
	}

	headers := make(map[string]string)
	if erClientOpts.JWTToken != "" {
		headers["Authorization"] = "Bearer " + erClientOpts.JWTToken
	}
	headers["Content-Type"] = "application/json"

	return &EventsRunnerClient{
		eventsRunnerBaseURL: erClientOpts.EventsRunnerBaseURL,
		httpClient:          httpClient,
		headers:             headers,
	}, nil
}

func (er EventsRunnerClient) ProcessEvent(event *eventqueue.Event) error {
	eventJson, err := json.Marshal(event)
	if err != nil {
		return err
	}
	requestURI := fmt.Sprintf("%s/api/v1/events", er.eventsRunnerBaseURL)
	req, err := http.NewRequest("POST", requestURI, bytes.NewBuffer(eventJson))
	if err != nil {
		return err
	}
	for k, v := range er.headers {
		req.Header.Set(k, v)
	}
	resp, err := er.httpClient.Do(req)
	if err != nil {
		return err
	}
	fmt.Println("response Status:", resp.StatusCode)
	if resp.StatusCode < 200 && resp.StatusCode >= 300 {
		fmt.Println("came")
		return errors.New("failed to process event")
	}
	return nil
}
