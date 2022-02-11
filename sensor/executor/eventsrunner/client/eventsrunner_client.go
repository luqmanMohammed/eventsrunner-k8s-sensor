package client

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

	"github.com/luqmanMohammed/eventsrunner-k8s-sensor/sensor/config"
	"github.com/luqmanMohammed/eventsrunner-k8s-sensor/sensor/eventqueue"
	"k8s.io/klog/v2"
)

// AuthType to determine the authentication type
type AuthType string

const (
	// mTLS is the authentication type for mutual TLS authentication
	mTLS AuthType = "mTLS"
	// JWT is the authentication type for JWT authentication
	JWT AuthType = "jwt"
)

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

// EventsRunnerClient holds an http.Client and required headers
// to be used for sending requests to the EventsRunner
type EventsRunnerClient struct {
	eventsRunnerBaseURL string
	httpClient          *http.Client
	headers             map[string]string
}

// createTLSConfig creates a TLS config based on the provided CA cert path,
// client key path and client cert path.
// If Client parameters are empty, only CA will be used to create the TLS
// config, which is usefull for HTTPS JWT Authentication
func createTLSConfig(caCertPath, clientKeyPath, clientCertPath string) (*tls.Config, error) {
	klog.V(2).Infof("Reading CA cert from %s", caCertPath)
	caCert, err := ioutil.ReadFile(caCertPath)
	if err != nil {
		klog.V(2).ErrorS(err, "Failed to read CA cert")
		return nil, err
	}
	caCertPool := x509.NewCertPool()
	if ok := caCertPool.AppendCertsFromPEM(caCert); !ok {
		klog.V(2).ErrorS(fmt.Errorf("failed to append CA cert"), "Failed to create CA cert pool")
		return nil, errors.New("failed to setup tls config")
	}
	tlsConfig := &tls.Config{
		RootCAs: caCertPool,
	}
	if clientCertPath != "" && clientKeyPath != "" {
		klog.V(2).Infof("Reading client cert from %s", clientCertPath)
		klog.V(2).Infof("Reading client key from %s", clientKeyPath)
		clientCert, err := tls.LoadX509KeyPair(clientCertPath, clientKeyPath)
		if err != nil {
			klog.V(2).ErrorS(err, "Failed to create client tls config")
			return nil, err
		}
		tlsConfig.Certificates = []tls.Certificate{clientCert}
	}
	return tlsConfig, nil
}

// newMutualTLSAuthClient creates a new EventsRunnerClient with mutual TLS authentication.
// Configures the client to use the provided CA cert path, client key path and client cert path.
// If JWT Token is provided, it will be added in the request Authorization header.
func newMutualTLSClient(erClientOpts *EventsRunnerClientOpts) (*EventsRunnerClient, error) {
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
		klog.V(2).Infof("Adding JWT token to request Authorization header since its provided")
		headers["Authorization"] = "Bearer " + erClientOpts.JWTToken
	}
	headers["Content-Type"] = "application/json"
	return &EventsRunnerClient{
		eventsRunnerBaseURL: erClientOpts.EventsRunnerBaseURL,
		httpClient:          httpClient,
		headers:             headers,
	}, nil
}

// newJWTAuthClient creates a new EventsRunnerClient with JWT authentication.
// Requires the JWT Token to be provided.
// If the server is HTTPS, CACertPath Option is required, the client will use
// the provided CA cert for server certificate verification.
// If all required options are present, set tryMTLS flag to true, client will
// try to initialize a mutual TLS client with JWT Token in the request Authorization
// header.
func newJWTClient(erClientOpts *EventsRunnerClientOpts, tryMTLS bool, httpsEndpoint bool) (*EventsRunnerClient, error) {
	var tlsConfig *tls.Config
	if tryMTLS {
		klog.V(2).Infof("Trying to initialize a mutual TLS client")
		var err error
		tlsConfig, err = createTLSConfig(erClientOpts.CaCertPath, erClientOpts.ClientKeyPath, erClientOpts.ClientCertPath)
		if err != nil {
			klog.V(2).Infof("TLS config was provided but failed to create: %v", err)
		}
	} else if httpsEndpoint {
		klog.V(2).Info("Endpoint identified to be HTTPS, CA cert path is required")
		var err error
		tlsConfig, err = createTLSConfig(erClientOpts.CaCertPath, "", "")
		if err != nil {
			return nil, err
		}
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

// New creates a new EventsRunnerClient with the provided options.
// Authentication mode is determined by the authType argument.
// If EventsRunnerBaseURL is an HTTPS endpoint, CACertPath Option is required even
// if JWT authentication is used.
// Check EventsRunnerClientOpts for each authentication methods requirements.
func New(authType AuthType, erClientOpts *EventsRunnerClientOpts) (*EventsRunnerClient, error) {
	mTLSRequirementsErr := config.AnyRequestedConfigMissing(map[string]interface{}{
		"CaCertPath":     erClientOpts.CaCertPath,
		"ClientKeyPath":  erClientOpts.ClientKeyPath,
		"ClientCertPath": erClientOpts.ClientCertPath,
	})
	if authType == "" {
		return nil, &config.RequiredConfigMissingError{ConfigName: "authType"}
	}
	klog.V(2).Infof("checking requirements for EventsRunner Client with authType: %s", authType)
	if basicRequirementErr := config.AnyRequestedConfigMissing(map[string]interface{}{
		"EventsRunnerBaseURL": erClientOpts.EventsRunnerBaseURL,
	}); basicRequirementErr != nil {
		return nil, basicRequirementErr
	}
	if authType == JWT {
		if jwtRequirementsErr := config.AnyRequestedConfigMissing(map[string]interface{}{
			"JWTToken": erClientOpts.JWTToken,
		}); jwtRequirementsErr != nil {
			return nil, jwtRequirementsErr
		}
		if strings.HasPrefix(erClientOpts.EventsRunnerBaseURL, "https") {
			if httpsRequirementsErr := config.AnyRequestedConfigMissing(map[string]interface{}{
				"CaCertPath": erClientOpts.CaCertPath,
			}); httpsRequirementsErr != nil {
				return nil, httpsRequirementsErr
			}
		}
		return newJWTClient(erClientOpts, mTLSRequirementsErr == nil, strings.HasPrefix(erClientOpts.EventsRunnerBaseURL, "https"))

	} else if authType == mTLS {
		if mTLSRequirementsErr != nil {
			return nil, mTLSRequirementsErr
		}
		return newMutualTLSClient(erClientOpts)
	}
	return nil, fmt.Errorf("authType %s is not supported", authType)
}

// ProcessEvent sends the event to the EventsRunner server, and will wait
// for a response.
// Only a Response code 200 (Subject to Change) is considered a success.
// Requests will be sent to "<base url>/api/v1/events".
func (er EventsRunnerClient) ProcessEvent(event *eventqueue.Event) error {
	klog.V(3).Infof("Processing event with rule ID: %s", event.RuleID)
	eventJSON, err := json.Marshal(event)
	if err != nil {
		klog.V(3).ErrorS(err, "Failed to marshal event")
		return err
	}
	requestURI := fmt.Sprintf("%s/api/v1/events", er.eventsRunnerBaseURL)
	req, err := http.NewRequest("POST", requestURI, bytes.NewBuffer(eventJSON))
	if err != nil {
		klog.V(3).ErrorS(err, "Failed to create request")
		return err
	}
	for k, v := range er.headers {
		req.Header.Set(k, v)
	}
	klog.V(3).Infof("Sending request to %s for event with rule ID", requestURI, event.RuleID)
	resp, err := er.httpClient.Do(req)
	if err != nil {
		klog.V(3).ErrorS(err, "Failed to send request")
		return err
	}
	klog.V(3).Infof("Got response of %s for event with rule ID: %s", event.RuleID, resp.Status)
	if resp.StatusCode != 200 {
		klog.V(3).ErrorS(fmt.Errorf("failed due to response code %d", resp.StatusCode), "Failed to process event")
		return fmt.Errorf("failed to process event. Got status %d", resp.StatusCode)
	}
	return nil
}
