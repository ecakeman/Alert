package httpprobe

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ecakeman/alertflow/services/system-core/internal/system"
)

func TestHTTPClassifiesFailures(t *testing.T) {
	ok := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(strings.Repeat("x", 5000)))
	}))
	defer ok.Close()
	sample := Probe(t.Context(), system.DueTarget{TargetType: "HTTP", Address: ok.URL, Timeout: time.Second})
	if !sample.Success || sample.StatusClass != "2xx" || len(sample.Raw) == 0 {
		t.Fatalf("success sample = %+v", sample)
	}
	if !strings.Contains(string(sample.Raw), strings.Repeat("x", 4096)) || strings.Contains(string(sample.Raw), strings.Repeat("x", 4097)) {
		t.Fatal("body limit is not 4096")
	}

	status := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer status.Close()
	sample = Probe(t.Context(), system.DueTarget{Address: status.URL, Timeout: time.Second})
	if sample.Success || sample.ErrorClass != "http_status" || sample.StatusClass != "5xx" {
		t.Fatalf("http status = %+v", sample)
	}

	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
	}))
	defer slow.Close()
	sample = Probe(t.Context(), system.DueTarget{Address: slow.URL, Timeout: 20 * time.Millisecond})
	if sample.ErrorClass != "timeout" {
		t.Fatalf("timeout class = %q", sample.ErrorClass)
	}

	sample = Probe(t.Context(), system.DueTarget{Address: "http://127.0.0.1:1", Timeout: 200 * time.Millisecond})
	if sample.ErrorClass != "connect" {
		t.Fatalf("connect class = %q", sample.ErrorClass)
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	cert := mustCert(t)
	tlsServer := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	tlsServer.Listener = ln
	tlsServer.TLS = &tls.Config{Certificates: []tls.Certificate{cert}}
	tlsServer.StartTLS()
	defer tlsServer.Close()
	sample = Probe(t.Context(), system.DueTarget{Address: tlsServer.URL, Timeout: time.Second})
	if sample.ErrorClass != "tls" {
		t.Fatalf("tls class = %q raw=%s", sample.ErrorClass, sample.Raw)
	}
}

func mustCert(t *testing.T) tls.Certificate {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "alertflow.test"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		DNSNames:     []string{"alertflow.test"},
	}
	der, err := x509.CreateCertificate(rand.Reader, tpl, tpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}
}
