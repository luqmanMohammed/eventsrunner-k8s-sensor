# EventsRunner-K8s-Sensor <!-- omit in toc -->

[![Go Report Card](https://goreportcard.com/badge/github.com/luqmanMohammed/eventsrunner-k8s-sensor)](https://goreportcard.com/report/github.com/luqmanMohammed/eventsrunner-k8s-sensor)
![GitHub Workflow Status](https://img.shields.io/github/workflow/status/luqmanMohammed/eventsrunner-k8s-sensor/Release?logo=github)
![GitHub Workflow Status](https://img.shields.io/github/workflow/status/luqmanMohammed/eventsrunner-k8s-sensor/CodeQL?label=CodeQL&logo=Github)
[![codecov](https://codecov.io/gh/luqmanMohammed/eventsrunner-k8s-sensor/branch/main/graph/badge.svg?token=8O5TTEUUP8)](https://codecov.io/gh/luqmanMohammed/eventsrunner-k8s-sensor)

Config driven sensor for Kubernetes events.

---

## Table of Contents <!-- omit in toc -->

- [1. What is it?](#1-what-is-it)
- [2. Build](#2-build)
- [3. Deployment](#3-deployment)
  - [3.1. Kubernetes](#31-kubernetes)
  - [3.2. Helm](#32-helm)
- [4. Configuration](#4-configuration)
  - [4.1. Sensor configuration](#41-sensor-configuration)
  - [4.2. Rules](#42-rules)
    - [4.2.1. Rule Normalization and Validation](#421-rule-normalization-and-validation)
    - [4.2.2. Rule collection from ConfigMaps](#422-rule-collection-from-configmaps)
- [5. Executor (Processor/Action)](#5-executor-processoraction)
  - [5.1. log executor](#51-log-executor)
  - [5.2. script executor](#52-script-executor)
  - [5.3. eventsrunner executor](#53-eventsrunner-executor)
    - [5.3.1. mTLS authentication](#531-mtls-authentication)
    - [5.3.2. JWT authentication](#532-jwt-authentication)
- [6. License](#6-license)

---

## 1. What is it?

[Events Runner](https://github.com/luqmanMohammed/eventsrunner) is a config-driven automation server that can be used to do event-driven automation where it follows the sensor-runner architecture. Sensors are intended to send events from several sources to the server utilizing its Rest API. Events Runner will process the received events and trigger actions based on configs.

This project implements a config-driven sensor to listen to Kubernetes events and trigger actions. The sensor is implemented in Go and uses the [k8s.io/client-go](https://pkg.go.dev/k8s.io/client-go) library to interface with Kubernetes. The sensor can be configured using the Rules construct to define what k8s events to listen to. The actions taken by the sensor can be either forwarding the event to Events Runner or running a script.

---

## 2. Build

## 3. Deployment

For every new release, a new image will be built, scanned and pushed to the registry at [Docker Registry](https://hub.docker.com/r/luqmanmohammed/eventsrunner-k8s-sensor). The image will be tagged with the version number of the release.

```shell
docker pull luqmanmohammed/eventsrunner-k8s-sensor:<release tag>
```

### 3.1. Kubernetes

> Caution: this cluster role provides access to all resources in the cluster.
> An all-access cluster role is not recommended for production use.
> This cluster role is intended for testing purposes only.
> Provide access only to the resources you want to listen in a production environment.
> Skip ClusterRole altogether if you want listen to the resources only in a specific namespace, go with a Role.

The sensor can be deployed using bare kubernetes resources. Inspect the `kubernetes` directory for more information.

```shell
kubectl apply -f kubernetes/
```

Once the sensor is deployed, you can create rules to define what events to listen to. Checkout the example rules in the examples directory.
Sensor would automatically register new rules when they are created. But if your making any changes to the sensor configuration, make sure
to restart the sensor.

### 3.2. Helm

TBA

## 4. Configuration

### 4.1. Sensor configuration

>[Viper](https://github.com/spf13/viper) is used to manage sensor configuration.

Sensor configuration will be collected from the following sources in the following order:

1. Defaults
2. Configuration file
    - Configuration file must be named `config.yaml`
    - Config file will be loaded from the following locations in the following order.
        1. /etc/er-k8s-sensor/config.yaml
        2. $HOME/.er-k8s-sensor/config.yaml
    - Config file location can be explicitly specified using the `--config` flag.
    - Configuration file must be in YAML format
3. Environment variables
    - Environment variables must have the prefix `ER_K8S_`
4. Command line arguments

Configs used by the sensor are:

|         **Key**          | **Type** |                                                                **Description**                                                                 |       **Default**        |
| :----------------------: | :------: | :--------------------------------------------------------------------------------------------------------------------------------------------: | :----------------------: |
|       LogVerbosity       |   int    |                                          Log verbosity to be used. Can be set using the `-v` flag too                                          |           None           |
|        SensorName        |  string  |              Name of the sensor, used to ignore specific objects from being processed by labeling them with `<SensorName>=ignore`              |      er-k8s-sensor       |
|      KubeConfigPath      |  string  |                                                       Absolute path to Kubernetes config                                                       |       $HOME/.kube        |
|     SensorNamespace      |  string  |                                                  Rules will be collected from this namespace                                                   |       eventsrunner       |
| SensorRuleConfigMapLabel |  string  |                                           Label to identify Rules configmaps in the SensorNamespace                                            | er-k8s-sensor-rules=true |
|       WorkerCount        |   int    |                                         Number of workers to be started to process items in the queue                                          |            10            |
|       MaxTryCount        |   int    |                                           Number of times failed events executions should be retried                                           |            5             |
|       RequeueDelay       |   time   |                                                  Time to wait before requeuing a failed event                                                  |           30s            |
|       ExecutorType       |  string  |                Type of executor to be used to process events in the queue. Valid options are `log`, `script` or `eventsrunner`                 |           log            |
|        ScriptDir         |  string  |                                    If script executor is used, Absolute path to the location of the scripts                                    |           None           |
|       ScriptPrefix       |  string  |                                        If script executor is used, Prefix that all scripts should have                                         |           None           |
|         AuthType         |  string  | If eventsrunner executor is used, Type of authentication to use when connecting to the Events Runner server. Valid options are `jwt` or `mTLS` |           None           |
|   EventsRunnerBaseURL    |  string  |                                       If eventsrunner executor is used, URL of the Events Runner server                                        |           None           |
|      RequestTimeout      |   time   |                      If eventsrunner executor is used, Request timeout to be configured when calling Events Runner server                      |           None           |
|        CaCertPath        |  string  |                If eventsrunner executor is used and auth type mTLS and/or HTTPS endpoint is used, Absolute path to the CA cert                 |           None           |
|         JWTToken         |  string  |                 If eventsrunner executor is used and auth type JWT is used, JWT token to be added in the authorization header                  |           None           |
|      ClientCertPath      |  string  |                         If eventsrunner executor is used and auth type mTLS  is used, Absolute path to the client cert                         |           None           |
|      ClientKeyPath       |  string  |                          If eventsrunner executor is used and auth type mTLS is used, Absolute path to the client key                          |           None           |

Example config file to be provided to the sensor when using the script executor (Refer above table to see all supported configs):

```yaml
---
sensorNamespace: "eventsrunner"
executorType: "script"
scriptDir:    "/tmp/test-scripts"
scriptPrefix: "script"
```

### 4.2. Rules

Rules are defined in the JSON format.

```json
{
    "id": "cm-rule-1",
    "group": "",
    "version": "v1",
    "resource": "configmaps",
    "namespaces": ["default"],
    "eventTypes": ["ADDED", "MODIFIED"],
    "updatesOn": ["data"],
    "fieldFilter": "metadata.name=test",
    "labelFilter": "testKey=testValue",
}
```

- `id` (Required): Identifier of the rule. **Should be unique across the entire sensor**. If multiple have the same id, the last one will be used. If an id is not provided, the rule will be ignored. The above example rule has the id `cm-rule-1`.
- `group` (Required): Group of the resource to be watched. The above example rule has the group `` since ConfigMap is part of the core API group.
- `version` (Required): API Version of the resource to be watched. Example rule has the version `v1` since ConfigMaps are part of APIVersion v1.
- `resource` (Required): Resource to be watched. The above example rule is set to watch the resource `configmaps`.
- `namespace` (Optional): List of namespaces where the resource should be watched. Remove namespaces for cluster-wide resources. If empty for namespace-bound resources, the sensor will watch resources in all namespaces. The above example rule has the namespaces `default`.
- `eventTypes` (Optional): List of event types to be considered. If empty, all event types will be considered. Valid options are `ADDED`, `MODIFIED`, `DELETED`.  The above example rule has the event types `ADDED` and `MODIFIED` configured.
- `updatesOn` (Optional): List of fields to be considered for updates. If empty, all fields will be considered. The above example shows that the sensor will process the event only if the `data` field is updated.
- `fieldFilter` (Optional): Filter to be applied on the field. If empty, no filter will be applied. All valid Kubernetes field selectors are supported. The above example shows that the sensor will consider the event if the `metadata.name` field is `test`
- `labelFilter` (Optional): Filter to be applied on the label. If empty, no filter will be applied. All valid Kubernetes label selectors are supported. The above example shows that the sensor will consider the event if the `testKey` label is present and the value is `testValue`

#### 4.2.1. Rule Normalization and Validation

> - If any of the rules are invalid, the sensor will ignore them and continue to process the rest of the rules.

Collected rules will be normalized and validated.

Following steps will be performed to normalize the rule:

1. Remove duplicates in the `eventTypes` list and convert the list to lowercase
2. Remove duplicates in the `updatesOn` list and convert the list to lowercase
3. Remove duplicates in the `namespaces` list and convert the list to lowercase
4. Remove namespaces if provided for cluster-wide resources

Following steps will be performed to validate the rule:

1. Validate that the `id` is not empty
2. Validate all items in the `eventTypes` list are valid. Valid options are `ADDED`, `MODIFIED` and `DELETED` (Lowercase is also accepted)
3. Validate that the resource identifier is valid by confirming that `group` and `version` are valid
4. Confirm that the configured resource is available in the cluster.

#### 4.2.2. Rule collection from ConfigMaps

Kubernetes Sensor supports loading rules from ConfigMaps. ConfigMaps in the `SensorNamespace` ([configurable](#31-sensor-configuration)) with the label `SensorRuleConfigMapLabel` ([configurable](#31-sensor-configuration)) will be considered as rules ConfigMaps. Rule ConfigMap's data field should have the key `rules` and the value should be a JSON array of rules JSON objects as depicted above. The sensor will automatically reload the affected rules if any of the rule ConfigMaps are updated.

---

## 5. Executor (Processor/Action)

Executor is the component that will be used to process the events. Event structure as follows:

```json
{
    "eventType": "added",
    "ruleID": "cm-rule-1",
    "objects": [
        {
            "kind": "ConfigMap",
        }
    ]
}
```

- `eventType`: Type of the event
- `ruleID`: Identifier of the rule that was triggered
- `objects`: List of objects that triggered the event
  - `ADDED`: The object that was added
  - `MODIFIED`: Pre-update object will be stored at index 0 and post-update object at index 1
  - `DELETED`: The object that was deleted

Specific executor can be configured using the `ExecutorType` ([configurable](#31-sensor-configuration)) config.

### 5.1. log executor

Log executor/action is the simplest executor which would log the rule's id. Intended to be used for debugging/testing/POC purposes.

### 5.2. script executor

> - :warning: Scripts executed by the sensor should be vetted before allowing the sensor to run it.
> - Required Configs: `ScriptDir` and `ScriptPrefix`

Script executor can be used to execute a script on the event inside the sensor environment. Scripts should be located in the `ScriptDir` ([configurable](#31-sensor-configuration)) directory. Scripts should have the following naming convention: `<ScriptPrefix>-<Rule.ID>.sh`. If the script is not valid nor executable, the execution will return an error. The relevant event will be passed to the script as an environment variable named `EVENT` which would be a base64 encoded JSON object. The executor would consider the execution a success; if the exit code is 0.

### 5.3. eventsrunner executor

> - NOTE: [Events Runner](https://github.com/luqmanMohammed/eventsrunner) is not ready for any use yet. Watch for updates.
> - Required Configs: `AuthType` and `EventsRunnerBaseURL`
> - Optional Configs: `RequestTimeout`

Events Runner executor can be used to forward the events to the Events Runner server to be processed. Executor supports both mTLS and JWT authentication methodologies to authenticate with the Events Runner server. Events are forwarded to the following endpoint of the Events Runner server: `<EventsRunnerBaseURL>/api/v1/events`.

Authentication methodology can be configured using the `AuthType` ([configurable](#31-sensor-configuration)) config.

#### 5.3.1. mTLS authentication

> - Required Configs: `CaCertPath`, `ClientCertPath` and `ClientKeyPath`

mTLS authentication is used to authenticate with the Events Runner server. The sensor will use the provided client cert and key to authenticate with the Events Runner server. The sensor will use the provided CA cert to validate the server's certificate.

#### 5.3.2. JWT authentication

> - Required Configs: `JWTToken`

JWT authentication is used to authenticate with the Events Runner server. The sensor will use the provided JWT token to authenticate with the Events Runner server. Token will be added as a Bearer token in the request Authorization header. The sensor will utilize the CA cert provided via `CACertPath` to validate the server's certificate if it's exposing a TLS enabled endpoint.

---

## 6. License

This software is licensed under the MIT license.
