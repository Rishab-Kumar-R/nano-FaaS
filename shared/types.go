package shared

type JobPayload struct {
	ExecutionID string `json:"executionId"`
	FunctionID  string `json:"functionId"`
	Language    string `json:"language"`
	Code        string `json:"code"`
	Input       string `json:"input,omitempty"`
	RetryCount  int    `json:"retryCount,omitempty"`
}

type LogEntry struct {
	Message   string `json:"message"`
	Level     string `json:"level"`
	Timestamp string `json:"timestamp"`
	Done      bool   `json:"done,omitempty"`
}

type ResultPayload struct {
	Output string `json:"output"`
	Error  string `json:"error,omitempty"`
}

type DeadLetterEntry struct {
	ExecutionID string `json:"executionId"`
	FunctionID  string `json:"functionId"`
	Language    string `json:"language"`
	Error       string `json:"error"`
	Retries     int    `json:"retries"`
}
