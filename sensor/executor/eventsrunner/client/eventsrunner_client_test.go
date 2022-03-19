package client

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/luqmanMohammed/eventsrunner-k8s-sensor/sensor/config"
	"github.com/luqmanMohammed/eventsrunner-k8s-sensor/sensor/eventqueue"
	"github.com/luqmanMohammed/eventsrunner-k8s-sensor/sensor/rules"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

var (
	testEvent = &eventqueue.Event{
		EventType: rules.ADDED,
		RuleID:    "test-rule",
		Objects: []*unstructured.Unstructured{
			{
				Object: map[string]interface{}{
					"apiVersion": "v1",
					"kind":       "Pod",
					"metadata": map[string]interface{}{
						"name": "test-pod",
					},
				},
			},
		},
	}

	testCaPKICertScript = `#!/bin/bash
set -xe
mkdir -p /tmp/test-pki

cat <<EOF> /tmp/test-pki/csr.conf
[ req ]
default_bits = 2048
prompt = no
default_md = sha256
req_extensions = req_ext
distinguished_name = dn

[ dn ]
C = US
ST = CA
L = CA
O = test
OU = test
CN = 127.0.0.1

[ req_ext ]
subjectAltName = @alt_names

[ alt_names ]
DNS.1 = localhost
DNS.2 = *.cluster.local
IP.1 = 127.0.0.1

[ v3_ext ]
authorityKeyIdentifier=keyid,issuer:always
basicConstraints=CA:FALSE
keyUsage=keyEncipherment,dataEncipherment
extendedKeyUsage=serverAuth,clientAuth
subjectAltName=@alt_names
EOF

openssl genrsa -out /tmp/test-pki/ca.key 2048
openssl req -x509 -new -nodes -key /tmp/test-pki/ca.key -subj "/CN=127.0.0.1" -days 10000 -out /tmp/test-pki/ca.crt

openssl genrsa -out /tmp/test-pki/server.key 2048
openssl req -new -key /tmp/test-pki/server.key -out /tmp/test-pki/server.csr -config /tmp/test-pki/csr.conf
openssl x509 -req -in  /tmp/test-pki/server.csr -CA /tmp/test-pki/ca.crt -CAkey /tmp/test-pki/ca.key -CAcreateserial -out /tmp/test-pki/server.crt -days 10000 -extensions v3_ext -extfile /tmp/test-pki/csr.conf

openssl genrsa -out /tmp/test-pki/client.key 2048
openssl req -new -key /tmp/test-pki/client.key -out /tmp/test-pki/client.csr -subj "/CN=Client"
openssl x509 -req -in  /tmp/test-pki/client.csr -CA /tmp/test-pki/ca.crt -CAkey /tmp/test-pki/ca.key -CAcreateserial -out /tmp/test-pki/client.crt -days 10000

`
)

func setupTLS() error {
	if _, err := os.Stat("/tmp/test-pki"); os.IsNotExist(err) {
		cmd := exec.Command("sh", "-c", testCaPKICertScript)
		file, err := os.OpenFile(os.DevNull, os.O_RDWR, os.ModeAppend)
		if err != nil {
			return err
		}
		cmd.Stdout = file
		cmd.Stderr = file
		time.Sleep(3 * time.Second)
		return cmd.Run()
	}
	return nil
}

func setupSetupMockServer(clientAuth tls.ClientAuthType, port string) *http.Server {
	caCert, err := ioutil.ReadFile("/tmp/test-pki/ca.crt")
	if err != nil {
		panic("Failed to read CA cert:" + err.Error())
	}
	caCertPool := x509.NewCertPool()
	caCertPool.AppendCertsFromPEM(caCert)
	tlsConfig := &tls.Config{
		ClientCAs:  caCertPool,
		ClientAuth: clientAuth,
	}
	mockMux := http.NewServeMux()
	mockMux.HandleFunc("/api/v1/events/", func(rw http.ResponseWriter, r *http.Request) {
		fmt.Println("Mock server got request")
		if r.Header.Get("Authorization") != "Bearer test-token" {
			rw.WriteHeader(http.StatusUnauthorized)
		}
		rw.WriteHeader(http.StatusOK)
	})
	server := &http.Server{
		Addr:      ":" + port,
		TLSConfig: tlsConfig,
		Handler:   mockMux,
	}
	return server
}

func TestMutualTLSAuthProcessEvent(t *testing.T) {
	http.DefaultServeMux = &http.ServeMux{}
	if err := setupTLS(); err != nil {
		t.Fatalf("Failed to setup TLS due to: %v", err)
	}
	ercOpts := EventsRunnerClientOpts{
		EventsRunnerBaseURL: "https://localhost:8080",
		CaCertPath:          "/tmp/test-pki/ca.crt",
		ClientKeyPath:       "/tmp/test-pki/client.key",
		ClientCertPath:      "/tmp/test-pki/client.crt",
		RequestTimeout:      time.Minute,
		JWTToken:            "test-token",
	}
	server := setupSetupMockServer(tls.RequireAndVerifyClientCert, "8080")
	go server.ListenAndServeTLS("/tmp/test-pki/server.crt", "/tmp/test-pki/server.key")
	time.Sleep(2 * time.Second)
	erClient, err := New(mTLS, &ercOpts)
	if err != nil {
		t.Fatalf("Failed to create MutualTLS client due to: %v", err)
	}
	if err = erClient.ProcessEvent(testEvent); err != nil {
		t.Fatalf("Failed to process event due to: %v", err)
	}
}

func TestJWTAuthProcessEvent(t *testing.T) {
	http.DefaultServeMux = &http.ServeMux{}
	if err := setupTLS(); err != nil {
		t.Fatalf("Failed to setup TLS due to: %v", err)
	}
	ercOpts := EventsRunnerClientOpts{
		EventsRunnerBaseURL: "https://localhost:8081",
		RequestTimeout:      time.Minute,
		JWTToken:            "test-secret",
		CaCertPath:          "/tmp/test-pki/ca2.crt",
		ClientKeyPath:       "/tmp/test-pki/client.key",
		ClientCertPath:      "/tmp/test-pki/client.crt",
	}
	server := setupSetupMockServer(tls.NoClientCert, "8081")
	mockMux := http.NewServeMux()
	mockMux.HandleFunc("/api/v1/events/", func(rw http.ResponseWriter, r *http.Request) {
		t.Log("JWT Mock server got request")
		if r.Header.Get("Authorization") != "Bearer test-secret" {
			rw.WriteHeader(http.StatusUnauthorized)
		} else {
			rw.WriteHeader(http.StatusOK)
		}
	})
	server.Handler = mockMux
	server.ErrorLog = log.New(os.Stdout, "", 0)
	go server.ListenAndServeTLS("/tmp/test-pki/server.crt", "/tmp/test-pki/server.key")
	time.Sleep(2 * time.Second)

	erClient, err := New(JWT, &ercOpts)
	if err == nil {
		t.Fatalf("Expected error due to invalid CA cert")
	}

	ercOpts.CaCertPath = "/tmp/test-pki/ca.crt"
	erClient, err = New(JWT, &ercOpts)
	if err != nil {
		t.Fatalf("Failed to create JWT client due to: %v", err)
	}
	if err = erClient.ProcessEvent(testEvent); err != nil {
		t.Fatalf("Failed to process event due to: %v", err)
	}
}

func TestJWTAuthWithMTLSProcessEvent(t *testing.T) {
	if err := setupTLS(); err != nil {
		t.Fatalf("Failed to setup TLS due to: %v", err)
	}
	ercOpts := EventsRunnerClientOpts{
		EventsRunnerBaseURL: "https://localhost:8082",
		RequestTimeout:      time.Minute,
		JWTToken:            "test-secret",
		CaCertPath:          "/tmp/test-pki/ca.crt",
	}
	server := setupSetupMockServer(tls.NoClientCert, "8082")
	mockMux := http.NewServeMux()
	mockMux.HandleFunc("/api/v1/events/", func(rw http.ResponseWriter, r *http.Request) {
		t.Log("JWT Mock server got request")
		if r.Header.Get("Authorization") != "Bearer test-secret" {
			rw.WriteHeader(http.StatusUnauthorized)
		} else {
			rw.WriteHeader(http.StatusOK)
		}
	})
	server.Handler = mockMux
	server.ErrorLog = log.New(os.Stdout, "", 0)
	go server.ListenAndServeTLS("/tmp/test-pki/server.crt", "/tmp/test-pki/server.key")
	time.Sleep(2 * time.Second)
	erClient, err := New(JWT, &ercOpts)
	if err != nil {
		t.Fatalf("Failed to create JWT client due to: %v", err)
	}
	if err = erClient.ProcessEvent(testEvent); err != nil {
		t.Fatalf("Failed to process event due to: %v", err)
	}
	ercOpts.JWTToken = "test-secret2"
	erClient, err = New(JWT, &ercOpts)
	if err != nil {
		t.Fatalf("Failed to create JWT client due to: %v", err)
	}
	if err = erClient.ProcessEvent(testEvent); err == nil {
		t.Fatalf("Expected error due to invalid JWT token")
	}
}

func TestClientErrorHandling(t *testing.T) {
	erClientOpts := &EventsRunnerClientOpts{
		EventsRunnerBaseURL: "https://localhost:8080",
		CaCertPath:          "",
		ClientKeyPath:       "",
		ClientCertPath:      "",
	}

	t.Run("should return error if auth type is invalid", func(t *testing.T) {
		if _, err := New("jwt", nil); err == nil {
			t.Fatal("Expected error but got nil")
		}
		if _, err := New("", erClientOpts); err == nil {
			t.Fatal("Expected error but got nil")
		}
		if _, err := New("invalid", erClientOpts); err == nil {
			t.Fatal("Expected error but got nil")
		}
	})

	t.Run("should return error if proper option is not present", func(t *testing.T) {
		erClientOpts.JWTToken = ""
		if _, err := New(JWT, erClientOpts); err == nil {
			t.Fatal("Expected error but got nil")
		}
		erClientOpts.JWTToken = "test-token"
		if _, err := New(JWT, erClientOpts); err == nil {
			t.Fatalf("Expected error but got nil")
		}
		if _, err := New(mTLS, erClientOpts); err == nil {
			t.Fatalf("Expected error but got nil")
		}
		erClientOpts.EventsRunnerBaseURL = ""
		if _, err := New(mTLS, erClientOpts); err == nil {
			t.Fatalf("Expected error but got nil")
		} else {
			errParsed := err.(*config.RequiredConfigMissingError)
			if errParsed.ConfigName != "EventsRunnerBaseURL" {
				t.Fatalf("Expected error to be %s but got %s", "EventsRunnerBaseURL", errParsed.ConfigName)
			}
		}
	})

	t.Run("should return error if TLS issue", func(t *testing.T) {
		erClientOpts.EventsRunnerBaseURL = "https://localhost:8080"
		erClientOpts.CaCertPath = "/tmp/test-pki/ca2.crt"
		erClientOpts.ClientKeyPath = "/tmp/test-pki/client.key"
		erClientOpts.ClientCertPath = "/tmp/test-pki/client.crt"

		// CA Cert not found
		if _, err := New(mTLS, erClientOpts); err == nil {
			t.Fatalf("Expected error but got nil")
		} else {
			if err.Error() != "open /tmp/test-pki/ca2.crt: no such file or directory" {
				t.Fatalf("Expected error to be %s but got %s", "open /tmp/test-pki/ca2.crt: no such file or directory", err.Error())
			}
		}

		// CA Cert is Invalid
		f, err := os.Create("/tmp/test-pki/ca2.crt")
		if err != nil {
			t.Fatalf("Failed to create file due to: %v", err)
		}
		defer func() {
			os.Remove("/tmp/test-pki/ca2.crt")
		}()
		if _, err := f.WriteString("test"); err != nil {
			t.Fatalf("Failed to write to file due to: %v", err)
		}
		if err := f.Close(); err != nil {
			t.Fatalf("Failed to close file due to: %v", err)
		}
		erClientOpts.CaCertPath = "/tmp/test-pki/ca2.crt"
		if _, err := New(mTLS, erClientOpts); err == nil {
			t.Fatalf("Expected error but got nil")
		} else {
			if err.Error() != "failed to setup tls config" {
				t.Fatalf("Expected error to be %s but got %s", "failed to setup tls config", err.Error())
			}
		}

		// Client Cert is Invalid
		erClientOpts.CaCertPath = "/tmp/test-pki/ca.crt"
		erClientOpts.ClientCertPath = "/tmp/test-pki/client2.crt"
		f, err = os.Create("/tmp/test-pki/client2.crt")
		if err != nil {
			t.Fatalf("Failed to create file due to: %v", err)
		}
		defer func() {
			os.Remove("/tmp/test-pki/client2.crt")
		}()
		if _, err := f.WriteString("test"); err != nil {
			t.Fatalf("Failed to write to file due to: %v", err)
		}
		if err := f.Close(); err != nil {
			t.Fatalf("Failed to close file due to: %v", err)
		}
		if _, err := New(mTLS, erClientOpts); err == nil {
			t.Fatalf("Expected error but got nil")
		} else {
			if err.Error() != "tls: failed to find any PEM data in certificate input" {
				t.Fatalf("Expected error to be %s but got %s", "tls: failed to find any PEM data in certificate input", err.Error())
			}
		}
	})
}
