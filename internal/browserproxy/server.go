package browserproxy

import (
	"bufio"
	"context"
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"
)

type Server struct {
	url        string
	upstream   *url.URL
	listener   net.Listener
	httpServer *http.Server
	transport  *http.Transport
	bypass     map[string]struct{}
	mu         sync.Mutex
	closed     bool
}

func Start(upstream string) (*Server, error) {
	parsed, err := url.Parse(strings.TrimSpace(upstream))
	if err != nil {
		return nil, fmt.Errorf("parse proxy url: %w", err)
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return nil, fmt.Errorf("proxy url must include scheme and host")
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("listen on local proxy port: %w", err)
	}

	srv := &Server{
		url:      "http://" + ln.Addr().String(),
		upstream: parsed,
		listener: ln,
		bypass:   defaultBypassHosts(),
	}
	srv.transport = &http.Transport{
		Proxy: http.ProxyURL(parsed),
	}
	if parsed.Scheme == "https" {
		srv.transport.TLSClientConfig = &tls.Config{MinVersion: tls.VersionTLS12}
	}
	srv.httpServer = &http.Server{
		Handler: http.HandlerFunc(srv.handle),
	}
	go func() {
		_ = srv.httpServer.Serve(ln)
	}()
	return srv, nil
}

func (s *Server) URL() string {
	if s == nil {
		return ""
	}
	return s.url
}

func (s *Server) Close() error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	s.closed = true
	if s.httpServer != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = s.httpServer.Shutdown(ctx)
	}
	if s.listener != nil {
		_ = s.listener.Close()
	}
	if s.transport != nil {
		s.transport.CloseIdleConnections()
	}
	return nil
}

func (s *Server) handle(w http.ResponseWriter, r *http.Request) {
	if strings.EqualFold(r.Method, http.MethodConnect) {
		s.handleConnect(w, r)
		return
	}
	s.handleHTTP(w, r)
}

func (s *Server) handleHTTP(w http.ResponseWriter, r *http.Request) {
	if s.isBypassHost(r.Host) {
		s.forwardDirect(w, r)
		return
	}
	targetURL := r.URL
	if targetURL == nil || targetURL.Scheme == "" || targetURL.Host == "" {
		targetURL = &url.URL{
			Scheme:   "http",
			Host:     r.Host,
			Path:     r.URL.Path,
			RawPath:  r.URL.RawPath,
			RawQuery: r.URL.RawQuery,
		}
	}
	req, err := http.NewRequestWithContext(r.Context(), r.Method, targetURL.String(), r.Body)
	if err != nil {
		http.Error(w, fmt.Sprintf("proxy request: %v", err), http.StatusBadGateway)
		return
	}
	req.Header = cloneHeaders(r.Header)
	req.Host = targetURL.Host
	copyHopByHopHeaders(req.Header)

	resp, err := s.transport.RoundTrip(req)
	if err != nil {
		http.Error(w, fmt.Sprintf("upstream proxy error: %v", err), http.StatusBadGateway)
		return
	}
	defer func() { _ = resp.Body.Close() }()

	copyResponseHeaders(w.Header(), resp.Header)
	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, resp.Body)
}

func (s *Server) handleConnect(w http.ResponseWriter, r *http.Request) {
	if s.isBypassHost(r.Host) {
		s.handleDirectConnect(w, r)
		return
	}
	hj, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "proxy hijack unsupported", http.StatusInternalServerError)
		return
	}

	targetConn, err := s.connectUpstream(r.Host)
	if err != nil {
		http.Error(w, fmt.Sprintf("proxy connect error: %v", err), http.StatusBadGateway)
		return
	}

	clientConn, _, err := hj.Hijack()
	if err != nil {
		_ = targetConn.Close()
		http.Error(w, "proxy hijack failed", http.StatusInternalServerError)
		return
	}

	_, _ = clientConn.Write([]byte("HTTP/1.1 200 Connection Established\r\n\r\n"))
	go tunnel(targetConn, clientConn)
	go tunnel(clientConn, targetConn)
}

func (s *Server) forwardDirect(w http.ResponseWriter, r *http.Request) {
	targetURL := r.URL
	if targetURL == nil || targetURL.Scheme == "" || targetURL.Host == "" {
		targetURL = &url.URL{
			Scheme:   "http",
			Host:     r.Host,
			Path:     r.URL.Path,
			RawPath:  r.URL.RawPath,
			RawQuery: r.URL.RawQuery,
		}
	}

	directTransport := &http.Transport{}
	req, err := http.NewRequestWithContext(r.Context(), r.Method, targetURL.String(), r.Body)
	if err != nil {
		http.Error(w, fmt.Sprintf("proxy request: %v", err), http.StatusBadGateway)
		return
	}
	req.Header = cloneHeaders(r.Header)
	req.Host = targetURL.Host
	copyHopByHopHeaders(req.Header)

	resp, err := directTransport.RoundTrip(req)
	if err != nil {
		http.Error(w, fmt.Sprintf("direct proxy error: %v", err), http.StatusBadGateway)
		return
	}
	defer func() { _ = resp.Body.Close() }()
	copyResponseHeaders(w.Header(), resp.Header)
	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, resp.Body)
}

func (s *Server) handleDirectConnect(w http.ResponseWriter, r *http.Request) {
	hj, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "proxy hijack unsupported", http.StatusInternalServerError)
		return
	}
	dst, err := net.Dial("tcp", r.Host)
	if err != nil {
		http.Error(w, fmt.Sprintf("direct connect error: %v", err), http.StatusBadGateway)
		return
	}
	clientConn, _, err := hj.Hijack()
	if err != nil {
		_ = dst.Close()
		http.Error(w, "proxy hijack failed", http.StatusInternalServerError)
		return
	}
	_, _ = clientConn.Write([]byte("HTTP/1.1 200 Connection Established\r\n\r\n"))
	go tunnel(dst, clientConn)
	go tunnel(clientConn, dst)
}

func (s *Server) connectUpstream(target string) (net.Conn, error) {
	host := s.upstream.Host
	if host == "" {
		return nil, fmt.Errorf("missing upstream host")
	}

	var conn net.Conn
	var err error
	switch s.upstream.Scheme {
	case "https":
		conn, err = tls.Dial("tcp", host, &tls.Config{ServerName: upstreamServerName(host), MinVersion: tls.VersionTLS12})
	default:
		conn, err = net.Dial("tcp", host)
	}
	if err != nil {
		return nil, err
	}

	req := &http.Request{
		Method: http.MethodConnect,
		URL:    &url.URL{Opaque: target},
		Host:   target,
		Header: make(http.Header),
	}
	if s.upstream.User != nil {
		username := s.upstream.User.Username()
		password, _ := s.upstream.User.Password()
		token := base64.StdEncoding.EncodeToString([]byte(username + ":" + password))
		req.Header.Set("Proxy-Authorization", "Basic "+token)
	}

	if err := req.Write(conn); err != nil {
		_ = conn.Close()
		return nil, err
	}
	br := bufio.NewReader(conn)
	resp, err := http.ReadResponse(br, req)
	if err != nil {
		_ = conn.Close()
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		_ = resp.Body.Close()
		_ = conn.Close()
		return nil, fmt.Errorf("upstream connect rejected with status %s", resp.Status)
	}
	_ = resp.Body.Close()
	return conn, nil
}

func tunnel(dst net.Conn, src net.Conn) {
	defer func() {
		_ = dst.Close()
		_ = src.Close()
	}()
	_, _ = io.Copy(dst, src)
}

func cloneHeaders(src http.Header) http.Header {
	dst := make(http.Header, len(src))
	for k, values := range src {
		dst[k] = append([]string(nil), values...)
	}
	return dst
}

func copyHopByHopHeaders(h http.Header) {
	for _, key := range []string{
		"Connection",
		"Proxy-Connection",
		"Keep-Alive",
		"Proxy-Authenticate",
		"Proxy-Authorization",
		"Te",
		"Trailer",
		"Transfer-Encoding",
		"Upgrade",
	} {
		h.Del(key)
	}
}

func copyResponseHeaders(dst, src http.Header) {
	for k, values := range src {
		switch strings.ToLower(k) {
		case "connection", "proxy-connection", "keep-alive", "proxy-authenticate", "proxy-authorization", "te", "trailer", "transfer-encoding", "upgrade":
			continue
		}
		for _, v := range values {
			dst.Add(k, v)
		}
	}
}

func upstreamServerName(host string) string {
	if h, _, err := net.SplitHostPort(host); err == nil {
		return h
	}
	return host
}

func defaultBypassHosts() map[string]struct{} {
	hosts := map[string]struct{}{
		"127.0.0.1": {},
		"localhost": {},
		"::1":       {},
		"0.0.0.0":   {},
		"::":        {},
	}
	if host := strings.TrimSpace(os.Getenv("HOST")); host != "" {
		hosts[host] = struct{}{}
	}
	if bypass := strings.TrimSpace(os.Getenv("PROXY_INTERNAL_BYPASS")); bypass != "" {
		for _, part := range strings.Split(bypass, ",") {
			if trimmed := strings.TrimSpace(part); trimmed != "" {
				hosts[trimmed] = struct{}{}
			}
		}
	}
	return hosts
}

func (s *Server) isBypassHost(host string) bool {
	if s == nil {
		return false
	}
	host = strings.TrimSpace(host)
	if host == "" {
		return false
	}
	hostOnly := host
	if h, _, err := net.SplitHostPort(host); err == nil {
		hostOnly = h
	}
	_, ok := s.bypass[hostOnly]
	return ok
}
