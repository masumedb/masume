package clickhouse

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/masumedb/masume/internal/db"
)

// failedAt matches the place a parse error stopped at, which the server counts from one.
var failedAt = regexp.MustCompile(`failed at position (\d+)`)

// serverFaultCodes are the errors that report a fault of the server, not of the statement.
var serverFaultCodes = map[int32]bool{
	// NETWORK_ERROR, SOCKET_TIMEOUT, TOO_MANY_SIMULTANEOUS_QUERIES, MEMORY_LIMIT_EXCEEDED,
	// TIMEOUT_EXCEEDED, and the read-only refusal.
	210: true, 209: true, 202: true, 241: true, 159: true, 164: true,
}

// serverError is an error the server reported, with its code.
type serverError struct {
	Code    int32
	Message string
}

func (reported *serverError) Error() string {
	return fmt.Sprintf("code %d: %s", reported.Code, reported.Message)
}

// serverErrorText matches the text of an error answer:
// `Code: 60. DB::Exception: Unknown table. (UNKNOWN_TABLE) (version 25.8.1.1 (official build))`.
var serverErrorText = regexp.MustCompile(`(?s)^Code: (\d+)\. (.*?)(?:\. \([A-Z0-9_]+\))?(?: \(version .*\))?\s*$`)

// parseServerError reads the code and the message out of an error answer of the server.
func parseServerError(text string) (*serverError, bool) {
	held := serverErrorText.FindStringSubmatch(strings.TrimSpace(text))
	if held == nil {
		return nil, false
	}
	code, err := strconv.ParseInt(held[1], 10, 32)
	if err != nil {
		return nil, false
	}
	return &serverError{Code: int32(code), Message: held[2]}, true
}

// findServerCode returns the code of an error the server reported.
func findServerCode(err error) (int32, bool) {
	var reported *serverError
	if !errors.As(err, &reported) {
		return 0, false
	}
	return reported.Code, true
}

// describeFailure returns the message of the server without its code.
func describeFailure(err error) string {
	var reported *serverError
	if errors.As(err, &reported) && reported.Message != "" {
		return trimServerPrefix(reported.Message)
	}
	return db.DescribeError(err)
}

// trimServerPrefix removes the node the message was received from, which names the server
// and not the fault.
func trimServerPrefix(message string) string {
	const marker = "DB::Exception: "
	if at := strings.LastIndex(message, marker); at != -1 {
		return message[at+len(marker):]
	}
	return message
}

// ReadStatementProblem returns the fault of a statement out of the error of the server. A
// fault of the server itself is not a fault of the statement. The prefix is the text the
// client wrote before the statement, which the place of the fault counts as well.
func ReadStatementProblem(err error, prefix string) (db.StatementProblem, bool) {
	var reported *serverError
	if !errors.As(err, &reported) || reported.Message == "" {
		return db.StatementProblem{}, false
	}
	if serverFaultCodes[reported.Code] {
		return db.StatementProblem{}, false
	}

	message := trimServerPrefix(reported.Message)
	problem := db.StatementProblem{Message: message}
	if offset, found := FindProblemOffset(message); found && offset >= len(prefix) {
		problem.Offset = offset - len(prefix)
		problem.HasOffset = true
	}
	return problem, true
}

// FindProblemOffset returns the offset in the statement the message points at.
func FindProblemOffset(message string) (int, bool) {
	held := failedAt.FindStringSubmatch(message)
	if held == nil {
		return 0, false
	}
	position, err := strconv.Atoi(held[1])
	if err != nil || position < 1 {
		return 0, false
	}
	// The server counts from one.
	return position - 1, true
}
