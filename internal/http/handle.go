package http

import (
	"bufio"
	"encoding/base64"
	"io"
	"net/http"
	"paqet/internal/flog"
	"paqet/internal/pkg/buffer"
	"strings"
)

func (h *HTTP) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if h.username != "" || h.password != "" {
		if !h.authenticate(r) {
			w.Header().Set("Proxy-Authenticate", `Basic realm="proxy"`)
			http.Error(w, "Proxy authentication required", http.StatusProxyAuthRequired)
			return
		}
	}

	if r.Method == http.MethodConnect {
		h.handleConnect(w, r)
	} else {
		h.handleHTTP(w, r)
	}
}

func (h *HTTP) authenticate(r *http.Request) bool {
	authHeader := r.Header.Get("Proxy-Authorization")
	if authHeader == "" {
		return false
	}

	const prefix = "Basic "
	if !strings.HasPrefix(authHeader, prefix) {
		return false
	}

	decoded, err := base64.StdEncoding.DecodeString(authHeader[len(prefix):])
	if err != nil {
		return false
	}

	creds := strings.SplitN(string(decoded), ":", 2)
	if len(creds) != 2 {
		return false
	}

	return creds[0] == h.username && creds[1] == h.password
}

func (h *HTTP) handleConnect(w http.ResponseWriter, r *http.Request) {
	flog.Infof("HTTP proxy accepted CONNECT %s -> %s", r.RemoteAddr, r.Host)

	hijacker, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "Hijacking not supported", http.StatusInternalServerError)
		return
	}

	clientConn, _, err := hijacker.Hijack()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer clientConn.Close()

	targetHost := r.Host
	if !strings.Contains(targetHost, ":") {
		targetHost = targetHost + ":443"
	}

	strm, err := h.client.TCP(targetHost)
	if err != nil {
		flog.Errorf("HTTP proxy failed to establish stream for %s -> %s: %v", r.RemoteAddr, targetHost, err)
		clientConn.Write([]byte("HTTP/1.1 502 Bad Gateway\r\n\r\n"))
		return
	}
	defer strm.Close()

	flog.Debugf("HTTP proxy stream %d established for CONNECT %s -> %s", strm.SID(), r.RemoteAddr, targetHost)
	clientConn.Write([]byte("HTTP/1.1 200 Connection Established\r\n\r\n"))

	errCh := make(chan error, 2)
	go func() {
		err := buffer.CopyT(clientConn, strm)
		errCh <- err
	}()
	go func() {
		err := buffer.CopyT(strm, clientConn)
		errCh <- err
	}()

	<-errCh
	flog.Debugf("HTTP proxy CONNECT %s -> %s closed", r.RemoteAddr, targetHost)
}

func (h *HTTP) handleHTTP(w http.ResponseWriter, r *http.Request) {
	flog.Infof("HTTP proxy accepted %s %s -> %s", r.Method, r.RemoteAddr, r.Host)

	targetHost := r.Host
	if !strings.Contains(targetHost, ":") {
		targetHost = targetHost + ":80"
	}

	strm, err := h.client.TCP(targetHost)
	if err != nil {
		flog.Errorf("HTTP proxy failed to establish stream for %s -> %s: %v", r.RemoteAddr, targetHost, err)
		http.Error(w, "Bad Gateway", http.StatusBadGateway)
		return
	}
	defer strm.Close()

	flog.Debugf("HTTP proxy stream %d established for %s %s -> %s", strm.SID(), r.Method, r.RemoteAddr, targetHost)

	removeHopByHopHeaders(r.Header)
	r.Header.Del("Proxy-Authorization")
	r.Header.Del("Proxy-Connection")

	outReq := r.Clone(r.Context())
	outReq.RequestURI = ""

	if err := outReq.Write(strm); err != nil {
		flog.Errorf("HTTP proxy failed to forward request to %s: %v", targetHost, err)
		http.Error(w, "Bad Gateway", http.StatusBadGateway)
		return
	}

	resp, err := http.ReadResponse(bufio.NewReader(strm), outReq)
	if err != nil {
		flog.Errorf("HTTP proxy failed to read response from %s: %v", targetHost, err)
		http.Error(w, "Bad Gateway", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	removeHopByHopHeaders(resp.Header)
	for key, values := range resp.Header {
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}
	w.WriteHeader(resp.StatusCode)

	io.Copy(w, resp.Body)
	flog.Debugf("HTTP proxy %s %s -> %s completed", r.Method, r.RemoteAddr, targetHost)
}

func removeHopByHopHeaders(header http.Header) {
	hopByHopHeaders := []string{
		"Connection",
		"Keep-Alive",
		"Proxy-Authenticate",
		"Proxy-Authorization",
		"TE",
		"Trailers",
		"Transfer-Encoding",
		"Upgrade",
	}
	for _, h := range hopByHopHeaders {
		header.Del(h)
	}
}
