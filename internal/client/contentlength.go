package client

import (
	"bufio"
	"bytes"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/go-faster/errors"
)

// WithContentLength puts value in the Content-Length header verbatim,
// replacing upstream's
//
//	headers = {'Content-Length': ''}
//	client.meta.events.register('before-call.s3.PutObject', add_headers)
//
// It cannot go through WithHeaders. net/http derives the framing headers from
// the request itself and drops whatever Content-Length is in Header, so an
// empty or negative value never reaches the wire. This swaps in a transport
// that writes the request by hand instead.
//
// The header is written after signing, as upstream's before-call hook is: the
// signature covers the length the SDK computed, which is the point — the
// server is being asked what it does with a request whose framing disagrees
// with itself.
func (f *Factory) WithContentLength(value string) func(*s3.Options) {
	return f.rawTransport(&value)
}

// WithoutContentLength sends no Content-Length header at all, replacing
// upstream's remove_header('Content-Length').
func (f *Factory) WithoutContentLength() func(*s3.Options) {
	return f.rawTransport(nil)
}

func (f *Factory) rawTransport(contentLength *string) func(*s3.Options) {
	return func(o *s3.Options) {
		o.HTTPClient = &rawClient{
			contentLength: contentLength,
			//nolint:gosec // ssl_verify = False is a supported config for test servers
			tls: &tls.Config{InsecureSkipVerify: !f.cfg.SSLVerify, MinVersion: tls.VersionTLS12},
		}
	}
}

// rawClient writes requests to the socket itself so the Content-Length header
// can say anything, or nothing.
type rawClient struct {
	// contentLength is the header value to send; nil omits the header.
	contentLength *string
	tls           *tls.Config
}

func (c *rawClient) Do(req *http.Request) (*http.Response, error) {
	var body []byte
	if req.Body != nil {
		b, err := io.ReadAll(req.Body)
		if err != nil {
			return nil, errors.Wrap(err, "read request body")
		}
		body = b
	}

	conn, err := c.dial(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = conn.Close() }()
	if deadline, ok := req.Context().Deadline(); ok {
		_ = conn.SetDeadline(deadline)
	}

	var buf bytes.Buffer
	fmt.Fprintf(&buf, "%s %s HTTP/1.1\r\n", req.Method, req.URL.RequestURI())
	fmt.Fprintf(&buf, "Host: %s\r\n", req.URL.Host)
	// Without a Content-Length the server cannot tell where this request
	// ends, so the connection is good for exactly one.
	buf.WriteString("Connection: close\r\n")
	for name, values := range req.Header {
		if strings.EqualFold(name, "Host") ||
			strings.EqualFold(name, "Connection") ||
			strings.EqualFold(name, "Content-Length") {
			continue
		}
		for _, v := range values {
			fmt.Fprintf(&buf, "%s: %s\r\n", name, v)
		}
	}
	if c.contentLength != nil {
		fmt.Fprintf(&buf, "Content-Length: %s\r\n", *c.contentLength)
	}
	buf.WriteString("\r\n")
	buf.Write(body)

	if _, err := conn.Write(buf.Bytes()); err != nil {
		return nil, errors.Wrap(err, "write request")
	}

	resp, err := http.ReadResponse(bufio.NewReader(conn), req)
	if err != nil {
		return nil, errors.Wrap(err, "read response")
	}
	// The connection closes when this returns, so the body has to be read
	// out first.
	payload, err := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if err != nil {
		return nil, errors.Wrap(err, "read response body")
	}
	resp.Body = io.NopCloser(bytes.NewReader(payload))
	return resp, nil
}

func (c *rawClient) dial(req *http.Request) (net.Conn, error) {
	addr := req.URL.Host
	if req.URL.Port() == "" {
		port := "80"
		if req.URL.Scheme == "https" {
			port = "443"
		}
		addr = net.JoinHostPort(addr, port)
	}

	conn, err := (&net.Dialer{}).DialContext(req.Context(), "tcp", addr)
	if err != nil {
		return nil, errors.Wrap(err, "dial")
	}
	if req.URL.Scheme != "https" {
		return conn, nil
	}

	cfg := c.tls.Clone()
	cfg.ServerName = req.URL.Hostname()
	tlsConn := tls.Client(conn, cfg)
	if err := tlsConn.HandshakeContext(req.Context()); err != nil {
		_ = conn.Close()
		return nil, errors.Wrap(err, "tls handshake")
	}
	return tlsConn, nil
}
