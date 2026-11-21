package main

import "time"

// DeployRequest representa o payload enviado ao endpoint POST /deploy do faas-agent.
type DeployRequest struct {
	FunctionName       string `json:"functionName"`
	ImageName          string `json:"imageName"`
	IdleTimeoutSeconds int    `json:"idleTimeoutSeconds,omitempty"`
	MaxReplicas        int    `json:"maxReplicas,omitempty"`
	MaxConcurrency     int    `json:"maxConcurrency,omitempty"`
}

// FunctionReplica representa os dados públicos de uma réplica retornados pelo faas-agent.
type FunctionReplica struct {
	ReplicaName    string    `json:"replicaName"`
	ContainerID    string    `json:"containerId"`
	HostPort       string    `json:"hostPort"`
	Status         string    `json:"status"`
	ActiveRequests int       `json:"activeRequests"`
	LastUsedAt     time.Time `json:"lastUsedAt"`
}

// FunctionInstance representa o estado consolidado de uma função no faas-agent.
type FunctionInstance struct {
	FunctionName       string             `json:"functionName"`
	ImageName          string             `json:"imageName"`
	Status             string             `json:"status"`
	ExecutionState     string             `json:"executionState"`
	ActiveRequests     int                `json:"activeRequests"`
	RunningReplicas    int                `json:"runningReplicas"`
	TotalReplicas      int                `json:"totalReplicas"`
	LastUsedAt         time.Time          `json:"lastUsedAt"`
	IdleTimeoutSeconds int                `json:"idleTimeoutSeconds"`
	MaxReplicas        int                `json:"maxReplicas"`
	MaxConcurrency     int                `json:"maxConcurrency"`
	Replicas           []*FunctionReplica `json:"replicas"`
}

// ExecutionLog representa um registro de execução retornado por GET /logs no faas-agent.
type ExecutionLog struct {
	ID                int64     `json:"id"`
	FunctionName      string    `json:"functionName"`
	ReplicaName       string    `json:"replicaName,omitempty"`
	StatusCode        int       `json:"statusCode"`
	RequestDurationMs int64     `json:"requestDurationMs"`
	Success           bool      `json:"success"`
	ColdStart         bool      `json:"coldStart"`
	ExecutedAt        time.Time `json:"executedAt"`
	ErrorMessage      string    `json:"errorMessage,omitempty"`
}
