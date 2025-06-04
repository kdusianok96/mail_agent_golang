package filter

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"os/exec"
	"strings"
	"time"
)

const (
	ScanResultClean    = "clean"
	ScanResultDetected = "detected"
	ScanResultError    = "error" // For script execution errors, not detection
)

const filterLogPrefix = "FILTER"

// ScanOutput represents the outcome of an email scan by an external script.
type ScanOutput struct {
	Result       string            // One of ScanResultClean, ScanResultDetected, ScanResultError
	HeadersToAdd map[string]string // Headers returned by the script's STDOUT
	Error        error             // Error encountered during script execution itself (timeout, not found, etc.)
	ExitCode     int               // Script's exit code (-1 if not available, e.g., due to timeout before start)
	StdErr       string            // Content of the script's STDERR
	StdOut       string            // Content of the script's STDOUT (primarily for debugging, headers are parsed into HeadersToAdd)
}

// ScanEmailWithScript executes an external script to scan email data.
// It passes emailData via STDIN and interprets results based on exit code and STDOUT.
func ScanEmailWithScript(emailData []byte, scriptPath string, scriptArgs []string, scriptTimeout time.Duration) ScanOutput {
	log.Printf("INFO: %s: Executing script '%s' with args %v and timeout %v", filterLogPrefix, scriptPath, scriptArgs, scriptTimeout)

	ctx, cancel := context.WithTimeout(context.Background(), scriptTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, scriptPath, scriptArgs...)

	cmd.Stdin = bytes.NewReader(emailData)

	var stdoutBuf, stderrBuf bytes.Buffer
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf

	output := ScanOutput{
		HeadersToAdd: make(map[string]string),
		ExitCode:     -1, // Default if ProcessState is nil (e.g. context timeout before command starts/finishes)
	}

	startTime := time.Now()
	err := cmd.Run() // Waits for command to finish or context to cancel
	executionTime := time.Since(startTime)

	// ProcessState might be nil if the command didn't start or was killed by timeout before exit status was available
	if cmd.ProcessState != nil {
		output.ExitCode = cmd.ProcessState.ExitCode()
		log.Printf("INFO: %s: Script '%s' execution finished in %v. Exit code: %d", filterLogPrefix, scriptPath, executionTime, output.ExitCode)
	} else {
		// This case can happen if the context deadline exceeds *before* cmd.Run() truly finishes setting up ProcessState
		// or if the process is killed by something other than a normal exit.
		log.Printf("INFO: %s: Script '%s' execution finished in %v. ProcessState is nil (may indicate pre-start timeout or kill signal).", filterLogPrefix, scriptPath, executionTime)
	}

	output.StdOut = strings.TrimSpace(stdoutBuf.String()) // Store raw stdout for debugging
	output.StdErr = strings.TrimSpace(stderrBuf.String())
	if output.StdErr != "" {
		log.Printf("INFO: %s: Script '%s' STDERR: %s", filterLogPrefix, scriptPath, output.StdErr)
	}
	if output.StdOut != "" { // Log raw stdout for debugging purposes
		log.Printf("DEBUG: %s: Script '%s' STDOUT: %s", filterLogPrefix, scriptPath, output.StdOut)
	}


	if ctx.Err() == context.DeadlineExceeded {
		log.Printf("ERROR: %s: Script '%s' timed out after %v (context deadline exceeded)", filterLogPrefix, scriptPath, scriptTimeout)
		output.Result = ScanResultError
		output.Error = fmt.Errorf("script '%s' execution timed out after %v", scriptPath, scriptTimeout)
		return output
	}

	if err != nil {
		// Error from cmd.Run()
		// This could be due to a non-zero exit code, or an issue like "executable file not found".
		if exitErr, ok := err.(*exec.ExitError); ok {
			// Script executed and returned a non-zero exit code.
			// We consider any non-zero exit code as "detected".
			// Specific exit codes could be handled differently if needed in the future.
			output.Result = ScanResultDetected
			output.Error = nil // Non-zero exit is a form of "successful execution" for detection purposes
			log.Printf("INFO: %s: Script '%s' indicated detection. Exit code: %d", filterLogPrefix, scriptPath, output.ExitCode)
		} else {
			// Other execution error (e.g., script not found, permissions error).
			log.Printf("ERROR: %s: Script '%s' execution failed: %v", filterLogPrefix, scriptPath, err)
			output.Result = ScanResultError
			output.Error = fmt.Errorf("script '%s' execution failed: %w", scriptPath, err)
			return output // Return early as STDOUT parsing is likely irrelevant
		}
	} else {
		// No error from cmd.Run(), means exit code 0.
		output.Result = ScanResultClean
		log.Printf("INFO: %s: Script '%s' indicated clean. Exit code: 0", filterLogPrefix, scriptPath)
	}

	// Parse STDOUT for headers, regardless of clean/detected, as a script might add info headers even for clean mail
	// or specific detection headers (e.g., X-Spam-Status).
	stdoutLines := strings.Split(output.StdOut, "\n")
	for _, line := range stdoutLines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, ":", 2)
		if len(parts) == 2 {
			headerName := strings.TrimSpace(parts[0])
			headerValue := strings.TrimSpace(parts[1])
			if headerName != "" && !strings.ContainsAny(headerName, " \t\r\n:") { // Basic header name validation
				output.HeadersToAdd[headerName] = headerValue
				log.Printf("DEBUG: %s: Script '%s' provided header for addition: '%s: %s'", filterLogPrefix, scriptPath, headerName, headerValue)
			} else {
				log.Printf("WARN: %s: Script '%s' STDOUT line ignored (invalid header name format): '%s'", filterLogPrefix, scriptPath, line)
			}
		} else {
			// Log lines that are not empty and not header-like, if they are unexpected.
			// This might be script debug output that mistakenly went to STDOUT.
			// For now, just ignore them as per the "one header per line" rule.
			// log.Printf("DEBUG: %s: Script '%s' STDOUT line not a key-value header: '%s'", filterLogPrefix, scriptPath, line)
		}
	}

	log.Printf("INFO: %s: Script '%s' final scan result: %s. Headers to add: %d.", filterLogPrefix, scriptPath, output.Result, len(output.HeadersToAdd))
	return output
}
