package scripts

import (
	"log"
	"os"
	"time"
)

// AuditLogger logs script executions for auditing purposes.
type AuditLogger struct {
	logger *log.Logger
}

// NewAuditLogger creates a new audit logger that writes to stderr.
func NewAuditLogger() *AuditLogger {
	return &AuditLogger{
		logger: log.New(os.Stderr, "[SCRIPTS AUDIT] ", log.LstdFlags),
	}
}

// Log records a script execution in the audit log.
func (a *AuditLogger) Log(ex *Execution) {
	a.logger.Printf(
		"name=%s status=%s exit_code=%d duration=%s client_ip=%s started_at=%s",
		ex.Name,
		ex.Status,
		ex.ExitCode,
		ex.Duration.Round(time.Millisecond),
		ex.ClientIP,
		ex.StartedAt.Format(time.RFC3339),
	)
}
