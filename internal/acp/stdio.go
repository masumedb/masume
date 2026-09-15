package acp

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"
	"sync"
)

// The transport reads and writes one JSON-RPC message per line, as the MCP stdio transport
// does.

// maxMessageBytes is the maximum length of one message from the agent.
const maxMessageBytes = 1 << 22

// The JSON-RPC error codes masume answers with.
const (
	codeMethodNotFound = -32601
	codeInternalError  = -32603
)

// rpcError is the error of one answer.
type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (err *rpcError) Error() string {
	return fmt.Sprintf("%s (%d)", err.Message, err.Code)
}

// message is one JSON-RPC message in either direction.
type message struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

// answer is the result of one call, or the error the agent returned.
type answer struct {
	result json.RawMessage
	err    error
}

// handlers are what the connection does with what the agent sends.
type handlers struct {
	// callMe answers one request of the agent. A nil result with a nil error is an empty
	// object.
	callMe func(method string, params json.RawMessage) (any, *rpcError)
	// notifyMe reads one notification of the agent.
	notifyMe func(method string, params json.RawMessage)
	// logLine reads one line of the traffic.
	logLine func(line string)
}

// connection is the JSON-RPC connection to one agent process.
type connection struct {
	output   io.Writer
	handlers handlers

	writeGuard sync.Mutex
	guard      sync.Mutex
	nextID     int64
	waiting    map[string]chan answer
	// closed is the reason the connection ended, once it has.
	closed error
}

// openConnection returns a connection that writes to the agent and reads its answers.
func openConnection(output io.Writer, held handlers) *connection {
	return &connection{
		output: output, handlers: held, waiting: map[string]chan answer{},
	}
}

// errConnectionClosed is the error of a call that the agent never answered.
var errConnectionClosed = errors.New("the agent closed the connection")

// readUntilEOF reads the agent until the stream ends, then fails every waiting call.
func (held *connection) readUntilEOF(input io.Reader) {
	reader := bufio.NewReaderSize(input, 64*1024)
	defer held.failEveryCall(errConnectionClosed)

	for {
		line, err := readMessageLine(reader)
		if trimmed := strings.TrimSpace(line); trimmed != "" {
			held.readOneMessage(trimmed)
		}
		if err != nil {
			return
		}
	}
}

// readMessageLine reads one line and drops a line above the message limit.
func readMessageLine(reader *bufio.Reader) (string, error) {
	held := strings.Builder{}
	tooLong := false
	for {
		part, err := reader.ReadSlice('\n')
		if held.Len()+len(part) > maxMessageBytes {
			tooLong = true
		}
		if !tooLong {
			held.Write(part)
		}
		if errors.Is(err, bufio.ErrBufferFull) {
			continue
		}
		if tooLong {
			return "", err
		}
		return held.String(), err
	}
}

// readOneMessage routes one message: an answer to a call, a request, or a notification.
func (held *connection) readOneMessage(line string) {
	held.handlers.logLine("< " + line)
	read := message{}
	if err := json.Unmarshal([]byte(line), &read); err != nil {
		held.handlers.logLine("! cannot parse the message: " + err.Error())
		return
	}

	// A message with a method is a request or a notification, whatever else it holds.
	if read.Method != "" {
		if len(read.ID) == 0 {
			held.handlers.notifyMe(read.Method, read.Params)
			return
		}
		held.answerOneRequest(read)
		return
	}
	if len(read.ID) == 0 {
		return
	}
	held.finishCall(string(read.ID), read)
}

// answerOneRequest runs one request of the agent and writes its answer.
func (held *connection) answerOneRequest(read message) {
	result, failure := held.handlers.callMe(read.Method, read.Params)
	written := message{JSONRPC: "2.0", ID: read.ID}
	if failure != nil {
		written.Error = failure
	} else {
		encoded, err := json.Marshal(result)
		if err != nil {
			written.Error = &rpcError{Code: codeInternalError, Message: err.Error()}
		} else {
			written.Result = encoded
		}
	}
	if err := held.writeMessage(written); err != nil {
		held.handlers.logLine("! cannot answer the agent: " + err.Error())
	}
}

// finishCall hands one answer to the call that waits for it.
func (held *connection) finishCall(id string, read message) {
	held.guard.Lock()
	room, waiting := held.waiting[id]
	delete(held.waiting, id)
	held.guard.Unlock()
	if !waiting {
		return
	}
	if read.Error != nil {
		room <- answer{err: read.Error}
		return
	}
	room <- answer{result: read.Result}
}

// failEveryCall ends every waiting call with this error.
func (held *connection) failEveryCall(reason error) {
	held.guard.Lock()
	defer held.guard.Unlock()
	held.closed = reason
	for id, room := range held.waiting {
		room <- answer{err: reason}
		delete(held.waiting, id)
	}
}

// writeMessage writes one message as a line. Two writers never interleave.
func (held *connection) writeMessage(written message) error {
	written.JSONRPC = "2.0"
	encoded, err := json.Marshal(written)
	if err != nil {
		return err
	}
	held.writeGuard.Lock()
	defer held.writeGuard.Unlock()
	held.handlers.logLine("> " + redactTokens(string(encoded)))
	_, err = held.output.Write(append(encoded, '\n'))
	return err
}

// bearerPattern matches the token of a header masume sends the agent.
var bearerPattern = regexp.MustCompile(`Bearer [A-Za-z0-9._\-]+`)

// redactTokens removes the token of the tool server from a logged line. The log records the
// traffic for a reader to follow, and a token in a file outlives the question it belongs to.
func redactTokens(line string) string {
	return bearerPattern.ReplaceAllString(line, "Bearer <redacted>")
}

// notify sends one notification, which has no answer.
func (held *connection) notify(method string, params any) error {
	encoded, err := json.Marshal(params)
	if err != nil {
		return err
	}
	return held.writeMessage(message{Method: method, Params: encoded})
}

// call sends one request and returns the channel its answer arrives on.
func (held *connection) call(method string, params any) (chan answer, error) {
	encoded, err := json.Marshal(params)
	if err != nil {
		return nil, err
	}

	held.guard.Lock()
	if held.closed != nil {
		reason := held.closed
		held.guard.Unlock()
		return nil, reason
	}
	held.nextID++
	id := fmt.Sprintf("%d", held.nextID)
	room := make(chan answer, 1)
	held.waiting[id] = room
	held.guard.Unlock()

	if err := held.writeMessage(message{
		ID: json.RawMessage(id), Method: method, Params: encoded,
	}); err != nil {
		held.guard.Lock()
		delete(held.waiting, id)
		held.guard.Unlock()
		return nil, err
	}
	return room, nil
}
