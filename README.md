# EventsRunner-K8s-Sensor
[![Go Report Card](https://goreportcard.com/badge/github.com/luqmanMohammed/eventsrunner-k8s-sensor)](https://goreportcard.com/report/github.com/luqmanMohammed/eventsrunner-k8s-sensor)
![example workflow](https://github.com/luqmanMohammed/eventsrunner-k8s-sensor/actions/workflows/build-and-test.yml/badge.svg)
![example workflow](https://github.com/luqmanMohammed/eventsrunner-k8s-sensor/actions/workflows/codeql-analysis.yml/badge.svg)

Config driven sensor for Kubernetes events.

---

## What is it?
[Events Runner](https://github.com/luqmanMohammed/eventsrunner) is a config driven automation server that can be used to do event-driven automation which follows the sensor-runner architecture. Sensors are  intended to send events from several sources to the Events Runner using the server's Rest API. Events Runner will process the received events and trigger actions based on configs.

This project implements a config driven sensor to listen on Kubernetes events and trigger actions. The sensor is implemented in Go and uses the [k8s.io/client-go](https://pkg.go.dev/k8s.io/client-go) to interface with Kubernetes. The sensor can be configured using the Rules construct to define what k8s events to listen on. The actions taken by sensor can be either forwarding the event to Events Runner or running a script.

---

## Usage
## Configuration

### Sensor configuration
>[Viper](https://github.com/spf13/viper) is used to manage sensor configuration.

Sensor configuration will be collected from the following > sources in the following order:

1. Defaults
2. Environment variables
    - Environment variables must have the prefix `ER_K8S_SENSOR_`
3. Configuration file
    - Configuration file must be named `config.yaml`
    - Config file will be loaded from the following locations in the following order.
        1. /etc/er-k8s-sensor/config.yaml
        2. $HOME/.er-k8s-sensor/config.yaml
    - Config file location can be explicitly specified using the `--config` flag.
    - Configuration file must be in YAML format

Configs used by the sensor are:

**Key**|**Type**|**Description**|**Default**
:-----:|:-----:|:-----:|:-----:
LogVerbosity|int|Log verbosity to be used|None
SensorName|string|Name of the sensor| used to ignore specific objects from being processed by labeling them with <SensorName>=ignore|er-k8s-sensor
KubeConfigPath|string|Absolute path to Kubernetes config|$HOME/.kube
SensorNamespace|string|Rules will be collected from this namespace|eventsrunner
SensorRuleConfigMapLabel|string|Label to identify Rules configmaps in the SensorNamespace|er-k8s-sensor-rules=true
WorkerCount|int|Number of workers to be started to process items in the queue|10
MaxTryCount|int|Number of times failed events executions should be retried|5
RequeueDelay|time|Time to wait before requeuing a failed event|30s
ExecutorType|string|Type of executor to be used to process events in the queue. Valid options are log| script
ScriptDir|string|If script executor is used| Absolute path to the location of the scripts
ScriptPrefix|string|If script executor is used| Prefix that all scripts should have
AuthType|string|If eventsrunner executor is used| Type of authentication to use when connecting to the Events Runner server
EventsRunnerBaseURL|string|If eventsrunner executor is used| URL of the Events Runner server
RequestTimeout|time|If eventsrunner executor is used| Request timeout to be configured when calling Events Runner server
CaCertPath|string|If eventsrunner executor is used and auth type mTLS and/or HTTPS endpoint is used| Absolute path to the CA cert
JWTToken|string|If eventsrunner executor is used and auth type JWT is used| JWT token to be added in the authorization header
ClientCertPath|string|If eventsrunner executor is used and auth type mTLS  is used| Absolute path to the client cert
ClientKeyPath|string|If eventsrunner executor is used and auth type mTLS is used| Absolute path to the client key