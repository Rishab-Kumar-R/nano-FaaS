package shared

type JobPayload struct {
	ExecutionID string `json:"executionId"`
	FunctionID  string `json:"functionId"`
	Language    string `json:"language"`
	Code        string `json:"code"`
}

type LogEntry struct {
	Message   string `json:"message"`
	Level     string `json:"level"`
	Timestamp string `json:"timestamp"`
	Done      bool   `json:"done,omitempty"`
}
