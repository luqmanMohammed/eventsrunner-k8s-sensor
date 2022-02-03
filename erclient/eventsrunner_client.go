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
	"strings"
	"time"

	"github.com/luqmanMohammed/eventsrunner-k8s-sensor/sensor/eventqueue"
	"k8s.io/klog/v2"
)

// RequiredFieldMissingError custom error is returned when required field is missing.
// Missing field is present as part of the error struct
type RequiredFieldMissingError struct {
	field string
}

// Error function implements error interface
func (rf *RequiredFieldMissingError) Error() string {
	return fmt.Sprintf("required field %s is missing", rf.field)
}

// EventsRunnerClientOpts is a struct that contains the options for the EventsRunnerClient
// If mTLS Auth Client is used, the following options are required:
// - CaCertPath: Path to the CA certificate
// - ClientCertPath: Path to the client certificate
// - ClientKeyPath: Path to the client key
// If JWT Auth Client is used, the following options are required:
// - JWTToken: JWT token
// In both cases, the following options are required:
// - EventsRunnerBaseURL: Base URL of the EventsRunner
type EventsRunnerClientOpts struct {
	EventsRunnerBaseURL string
	CaCertPath          string
	ClientKeyPath       string
	ClientCertPath      string
	RequestTimeout      time.Duration
	JWTToken            string
}

// EventsRunnerClient will be
type EventsRunnerClient struct {
	eventsRunnerBaseURL string
	httpClient          *http.Client
	headers             map[string]string
}

// validateBaseRequirements Validates that the base requirements for both
// authentication flows are met
func validateBaseRequirements(erClientOpts *EventsRunnerClientOpts) error {
	if erClientOpts == nil {
		return &RequiredFieldMissingError{field: "all"}
	}
	if erClientOpts.EventsRunnerBaseURL == "" {
		return &RequiredFieldMissingError{field: "EventsRunnerBaseURL"}
	}
	return nil
}

// validateMutualTLSAuthRequirements Validates that the required fields for
// mutual TLS authentication are present
func validateMutualTLSAuthRequirements(erClientOpts *EventsRunnerClientOpts) error {
	if erClientOpts.CaCertPath == "" {
		return &RequiredFieldMissingError{field: "CaCertPath"}
	}
	if erClientOpts.ClientCertPath == "" {
		return &RequiredFieldMissingError{field: "ClientCertPath"}
	}
	if erClientOpts.ClientKeyPath == "" {
		return &RequiredFieldMissingError{field: "ClientKeyPath"}
	}
	return nil
}

// createTLSConfig creates a TLS config based on the provided CA cert path,
// client key path and client cert path.
// If Client parameters are empty, only CA will be used to create the TLS
// config, which is usefull for HTTPS JWT Authentication
func createTLSConfig(caCertPath, clientKeyPath, clientCertPath string) (*tls.Config, error) {
	caCert, err := ioutil.ReadFile(caCertPath)
	if err != nil {
		return nil, err
	}
	caCertPool := x509.NewCertPool()
	if ok := caCertPool.AppendCertsFromPEM(caCert); !ok {
		return nil, errors.New("failed to setup tls config")
	}
	tlsConfig := &tls.Config{
		RootCAs: caCertPool,
	}
	if clientCertPath != "" && clientKeyPath != "" {
		clientCert, err := tls.LoadX509KeyPair(clientCertPath, clientKeyPath)
		if err != nil {
			return nil, err
		}
		tlsConfig.Certificates = []tls.Certificate{clientCert}
	}
	return tlsConfig, nil
}

// NewMutualTLSAuthClient creates a new EventsRunnerClient with mutual TLS authentication.
// Configures the client to use the provided CA cert path, client key path and client cert path.
// If JWT Token is provided, it will be added in the request Authorization header.
func NewMutualTLSClient(erClientOpts *EventsRunnerClientOpts) (*EventsRunnerClient, error) {
	if err := validateBaseRequirements(erClientOpts); err != nil {
		return nil, err
	}
	if err := validateMutualTLSAuthRequirements(erClientOpts); err != nil {
		return nil, err
	}
	tlsConfig, err := createTLSConfig(erClientOpts.CaCertPath, erClientOpts.ClientKeyPath, erClientOpts.ClientCertPath)
	if err != nil {
		return nil, err
	}
	httpClient := &http.Client{
		Timeout: erClientOpts.RequestTimeout,
		Transport: &http.Transport{
			TLSClientConfig: tlsConfig,
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

// NewJWTAuthClient creates a new EventsRunnerClient with JWT authentication.
// Requires the JWT Token to be provided.
// If the server is HTTPS, CACertPath Option is required, the client will use
// the provided CA cert for server certificate verification.
// If all required options for mTLS are provided, client will use them.
func NewJWTClient(erClientOpts *EventsRunnerClientOpts) (*EventsRunnerClient, error) {
	if err := validateBaseRequirements(erClientOpts); err != nil {
		return nil, err
	}
	var tlsConfig *tls.Config
	if err := validateMutualTLSAuthRequirements(erClientOpts); err == nil {
		tlsConfig, err = createTLSConfig(erClientOpts.CaCertPath, erClientOpts.ClientKeyPath, erClientOpts.ClientCertPath)
		if err != nil {
			klog.V(2).Infof("TLS config was provided but failed to create: %v", err)
		}
	} else if strings.HasPrefix(erClientOpts.EventsRunnerBaseURL, "https") {
		if erClientOpts.CaCertPath == "" {
			return nil, &RequiredFieldMissingError{field: "CaCertPath"}
		}
		var err error
		tlsConfig, err = createTLSConfig(erClientOpts.CaCertPath, "", "")
		if err != nil {
			return nil, err
		}
	}
	if erClientOpts.JWTToken == "" {
		return nil, &RequiredFieldMissingError{field: "JWTToken"}
	}
	headers := make(map[string]string)
	headers["Authorization"] = "Bearer " + erClientOpts.JWTToken
	headers["Content-Type"] = "application/json"
	httpClient := &http.Client{
		Timeout: erClientOpts.RequestTimeout,
		Transport: &http.Transport{
			TLSClientConfig: tlsConfig,
		},
	}
	return &EventsRunnerClient{
		eventsRunnerBaseURL: erClientOpts.EventsRunnerBaseURL,
		httpClient:          httpClient,
		headers:             headers,
	}, nil
}

// ProcessEvent sends an event to the EventsRunner server, and will wait
// for a response.
// Response code 200 (Subject to Change) is only considered a success.
// Requests will be sent to "<base url>/api/v1/events".
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
	if resp.StatusCode != 200 {
		return fmt.Errorf("failed to process event. Got status %d", resp.StatusCode)
	}
	return nil
}
