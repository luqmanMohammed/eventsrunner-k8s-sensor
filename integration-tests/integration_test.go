package integrationtests

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync/atomic"
	"testing"
)

var cmRuleCount, secretsRuleCount = new(int32), new(int32)

func PrepareAndRunJWTBasedMockServer() {
	mockServerMux := http.NewServeMux()
	mockServerMux.HandleFunc("/api/v1/events/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			if r.Header.Get("Authorization") == "Bearer test-token" {
				bodyMap := make(map[string]interface{})
				err := json.NewDecoder(r.Body).Decode(&bodyMap)
				if err != nil {
					w.WriteHeader(http.StatusBadRequest)
					return
				}
				ruleIDInt, ok := bodyMap["ruleId"]
				if !ok {
					w.WriteHeader(http.StatusBadRequest)
					return
				}
				ruleID := ruleIDInt.(string)
				if ruleID == "cm-rule-1" {
					atomic.AddInt32(cmRuleCount, 1)
				}
				if ruleID == "secrets-rule-1" {
					atomic.AddInt32(secretsRuleCount, 1)
				}
				w.WriteHeader(http.StatusCreated)
			} else {
				w.WriteHeader(http.StatusUnauthorized)
			}
		} else {
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})
	server := &http.Server{
		Addr:    ":8080",
		Handler: mockServerMux,
	}
	if err := server.ListenAndServe(); err != nil {
		panic(err)
	}
}

func TestInt(t *testing.T) {
	fmt.Println("Create Prequisite K8s Resources")
	fmt.Println("Get VM Ip")
	fmt.Println("Start Mock Server")
	fmt.Println("Create JWT-EventsRunner Sensor-config configmap. Set VM IP for BaseURL")
	fmt.Println("Create deployment for sensor by setting image tag as sha if CI")
	fmt.Println("Run seperate routine to check memory and cpu usage of the sensor")
	fmt.Println("Generate load by creating objects in k8s")
	fmt.Println("Wait for 30 seconds")
	fmt.Println("Check if all created objects are processed by the sensor")
}
