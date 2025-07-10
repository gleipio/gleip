package backend

import (
	"Gleip/backend/network"
	"net/http"
)

// InterceptQueue interface for managing intercepted requests (Single Responsibility)
type InterceptQueue interface {
	Add(transaction *network.HTTPTransaction) error
	Remove(id string) error
	Get(id string) (*network.HTTPTransaction, bool)
	GetAll() []*network.HTTPTransaction
	Clear()
}

// VariableExtractor interface for extracting variables from responses (Single Responsibility)
type VariableExtractor interface {
	Extract(extract VariableExtract, transaction *network.HTTPTransaction) (string, error)
}

// ScriptExecutor interface for executing scripts (Single Responsibility)
type ScriptExecutor interface {
	Execute(script string, context *ExecutionContext) (map[string]string, error)
}

// ResponseDecompressor interface for decompressing responses (Single Responsibility)
type ResponseDecompressor interface {
	Decompress(body []byte, contentEncoding string) ([]byte, error)
}

// VariableProcessor interface for processing variables in strings (Single Responsibility)
type VariableProcessor interface {
	ProcessVariables(input string, variables map[string]string) string
}

// TransactionEventEmitter interface for emitting transaction events (Single Responsibility)
type TransactionEventEmitter interface {
	EmitNewTransaction(transaction network.HTTPTransaction)
	EmitTransactionUpdate(transaction network.HTTPTransaction)
	EmitStepExecuted(gleipFlowId string, stepIndex int, results []ExecutionResult)
	EmitFuzzUpdate(stepId string, fuzzResults []FuzzResult)
}

// ProxyHandler interface for handling proxy requests (Interface Segregation)
type ProxyHandler interface {
	HandleHTTP(w http.ResponseWriter, r *http.Request)
	HandleConnect(w http.ResponseWriter, r *http.Request)
}

// CertificateProvider interface for certificate operations (Dependency Inversion)
type CertificateProvider interface {
	GenerateCertificate(hostname string) (interface{}, error)
	GetCertificateForConn(hello interface{}) (interface{}, error)
	GetCAPath() string
}

// StepExecutor interface for executing different types of steps (Open/Closed Principle)
type StepExecutor interface {
	Execute(step interface{}, ctx *ExecutionContext) ExecutionResult
	GetStepType() string
}

// RequestBuilder interface for building HTTP requests (Single Responsibility)
type RequestBuilder interface {
	BuildFromRaw(rawRequest string, host string, tls bool) (*http.Request, error)
	BuildFromComponents(method, url, body string, headers map[string]string) (*http.Request, error)
}

// ResponseFormatter interface for formatting responses (Single Responsibility)
type ResponseFormatter interface {
	FormatRequest(req *http.Request, body string) string
	FormatResponse(resp *http.Response, body []byte) string
}

// TransactionFactory interface for creating transactions (Factory Pattern)
type TransactionFactory interface {
	CreateTransaction(req network.HTTPRequest, resp *network.HTTPResponse) *network.HTTPTransaction
}

// ProxyServer interface (segregated from implementation)
type ProxyServerInterface interface {
	Start() error
	Stop() error
	SetInterceptEnabled(enabled bool)
	GetInterceptEnabled() bool
}

// ExecutionStrategy interface for different execution strategies (Strategy Pattern)
type ExecutionStrategy interface {
	Execute(gleipFlow *GleipFlow) ([]ExecutionResult, error)
}
