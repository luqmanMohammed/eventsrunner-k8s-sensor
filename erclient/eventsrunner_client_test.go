package erclient

import (
	"crypto/tls"
	"crypto/x509"
	"io/ioutil"
	"net/http"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/luqmanMohammed/eventsrunner-k8s-sensor/sensor/eventqueue"
	"github.com/luqmanMohammed/eventsrunner-k8s-sensor/sensor/rules"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

var (
	test_event = &eventqueue.Event{
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

	test_ca_pki_cert_script = `#!/bin/bash
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
	cmd := exec.Command("sh", "-c", test_ca_pki_cert_script)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func setupSetupMockServer(clientAuth tls.ClientAuthType) *http.Server {
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
	http.HandleFunc("/api/v1/events/", func(rw http.ResponseWriter, r *http.Request) {
		rw.WriteHeader(http.StatusOK)
	})
	server := &http.Server{
		Addr:      ":8080",
		TLSConfig: tlsConfig,
	}
	return server
}

func TestMTLSAuthProcessEvent(t *testing.T) {
	http.DefaultServeMux = &http.ServeMux{}
	if err := setupTLS(); err != nil {
		t.Fatalf("Failed to setup TLS due to: %v", err)
	}
	defer func() {
		os.RemoveAll("/tmp/test-pki")
	}()

	ercOpts := EventsRunnerClientOpts{
		EventsRunnerBaseURL: "https://localhost:8080",
		CaCertPath:          "/tmp/test-pki/ca.crt",
		ClientKeyPath:       "/tmp/test-pki/client.key",
		ClientCertPath:      "/tmp/test-pki/client.crt",
		RequestTimeout:      time.Minute,
	}
	server := setupSetupMockServer(tls.RequireAndVerifyClientCert)
	go server.ListenAndServeTLS("/tmp/test-pki/server.crt", "/tmp/test-pki/server.key")
	defer server.Close()
	erClient, err := NewMTLSClient(ercOpts)
	if err != nil {
		t.Fatalf("Failed to create MTLS client due to: %v", err)
	}
	if err = erClient.ProcessEvent(test_event); err != nil {
		t.Fatalf("Failed to process event due to: %v", err)
	}
}
