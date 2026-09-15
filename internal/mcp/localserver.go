package mcp

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

// The AI chat of the client serves its own tools to an agent over the loopback address. The
// tools run on the connection the chat is open on, so the agent reads what the reader reads.

// The limits of the loopback server.
const (
	// maxLocalBodyBytes is the largest message the agent can send.
	maxLocalBodyBytes = 1 << 22
	// localHeaderTimeout is the time one request has to send its headers.
	localHeaderTimeout = 30 * time.Second
	// localIdleTimeout is the time a connection of the agent can stand idle.
	localIdleTimeout = 5 * time.Minute
)

// localServerName is the name the agent knows this server by.
const localServerName = "masume"

// LocalServer serves the tools of the chat on the loopback address. It lives as long as one
// question does.
type LocalServer struct {
	server   *http.Server
	address  string
	token    string
	log      func(string)
	finished chan struct{}
}

// StartLocalServer opens the server on a port of the operating system and serves until the
// caller closes it.
func StartLocalServer(tools []Tool, version string, log func(string)) (*LocalServer, error) {
	// The loopback address alone answers, so nothing outside this machine reaches it.
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	token, err := buildLocalToken()
	if err != nil {
		_ = listener.Close()
		return nil, err
	}

	responder := CreateResponder(ResponderDeps{
		Tools: tools, Asker: CreateAsker(func(string) {}),
		Info:     ServerInfo{Name: localServerName, Version: version},
		LogEvent: log,
	})
	held := &LocalServer{
		address: "http://" + listener.Addr().String() + "/mcp",
		token:   token, log: log, finished: make(chan struct{}),
	}
	held.server = &http.Server{
		Handler:           held.buildHandler(responder),
		ReadHeaderTimeout: localHeaderTimeout,
		IdleTimeout:       localIdleTimeout,
	}

	go func() {
		defer close(held.finished)
		if err := held.server.Serve(listener); err != nil &&
			!errors.Is(err, http.ErrServerClosed) {
			log("! the tool server ended: " + err.Error())
		}
	}()
	return held, nil
}

// Address returns the address the agent sends to.
func (held *LocalServer) Address() string { return held.address }

// Token is the bearer token the agent sends. A request without it is refused, so another
// program on this machine cannot reach the connection of the chat.
func (held *LocalServer) Token() string { return held.token }

// Close ends the server and waits for it.
func (held *LocalServer) Close() error {
	ctx, stop := context.WithTimeout(context.Background(), time.Second)
	defer stop()
	err := held.server.Shutdown(ctx)
	<-held.finished
	return err
}

// buildLocalToken returns the bearer token of one server.
func buildLocalToken() (string, error) {
	held := make([]byte, 32)
	if _, err := rand.Read(held); err != nil {
		return "", err
	}
	return hex.EncodeToString(held), nil
}

// buildHandler answers one message per request, in the form of the streamable HTTP
// transport.
func (held *LocalServer) buildHandler(responder *Responder) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, asked *http.Request) {
		held.log("< " + asked.Method + " " + asked.URL.Path)
		// A browser sends the origin of the page. No MCP client does, so a request that
		// carries one comes from a page and is refused.
		if asked.Header.Get("origin") != "" {
			writer.WriteHeader(http.StatusForbidden)
			return
		}
		if !held.holdsToken(asked) {
			writer.WriteHeader(http.StatusUnauthorized)
			return
		}
		if asked.Method != http.MethodPost {
			// A GET opens the stream an agent listens on, and this server sends
			// nothing of its own.
			writer.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		body, err := io.ReadAll(io.LimitReader(asked.Body, maxLocalBodyBytes))
		if err != nil {
			writer.WriteHeader(http.StatusBadRequest)
			return
		}
		var message any
		if err := json.Unmarshal(body, &message); err != nil {
			writeLocalJSON(writer, buildError(nil, parseError,
				"the message is not valid JSON"))
			return
		}

		answer := responder.AnswerMessage(asked.Context(), message)
		if answer == nil {
			// A notification has no answer.
			writer.WriteHeader(http.StatusAccepted)
			return
		}
		writeLocalJSON(writer, answer)
	})
}

// holdsToken is true where the request carries the bearer token of this server.
func (held *LocalServer) holdsToken(asked *http.Request) bool {
	written := asked.Header.Get("authorization")
	return strings.TrimSpace(strings.TrimPrefix(written, "Bearer ")) == held.token
}

// writeLocalJSON writes one answer as JSON.
func writeLocalJSON(writer http.ResponseWriter, answer any) {
	written, err := json.Marshal(answer)
	if err != nil {
		writer.WriteHeader(http.StatusInternalServerError)
		return
	}
	writer.Header().Set("content-type", "application/json")
	_, _ = writer.Write(written)
}
