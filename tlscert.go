package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"log"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// certManager serves ADM's own certificate (so a Let's Encrypt certificate
// configured in ADM is reused) and falls back to a self-signed one.
type certManager struct {
	certFile, keyFile, pemFile string
	selfDir                    string
	log                        *log.Logger

	mu        sync.Mutex
	cert      *tls.Certificate
	loadedMod time.Time
	lastCheck time.Time
	self      *tls.Certificate
	warned    bool
}

func (m *certManager) GetCertificate(*tls.ClientHelloInfo) (*tls.Certificate, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if time.Since(m.lastCheck) > time.Minute {
		m.lastCheck = time.Now()
		m.reload()
	}
	if m.cert != nil {
		return m.cert, nil
	}
	if m.self == nil {
		c, err := loadOrCreateSelfSigned(m.selfDir)
		if err != nil {
			return nil, err
		}
		m.self = c
		m.log.Printf("kendinden imzalı sertifika kullanılıyor (%s)", m.selfDir)
	}
	return m.self, nil
}

func (m *certManager) reload() {
	if m.certFile == "" {
		return
	}
	st, err := os.Stat(m.certFile)
	if err != nil {
		m.cert = nil
		return
	}
	if m.cert != nil && st.ModTime().Equal(m.loadedMod) {
		return
	}
	c, err := tls.LoadX509KeyPair(m.certFile, m.keyFile)
	if err != nil && m.pemFile != "" {
		c, err = tls.LoadX509KeyPair(m.pemFile, m.pemFile)
	}
	if err != nil {
		if !m.warned {
			m.log.Printf("ADM sertifikası yüklenemedi (%v), kendinden imzalı sertifikaya geçiliyor", err)
			m.warned = true
		}
		m.cert = nil
		return
	}
	m.cert = &c
	m.loadedMod = st.ModTime()
	m.log.Printf("ADM sertifikası yüklendi: %s", m.certFile)
}

func loadOrCreateSelfSigned(dir string) (*tls.Certificate, error) {
	certPath := filepath.Join(dir, "self.crt")
	keyPath := filepath.Join(dir, "self.key")
	if c, err := tls.LoadX509KeyPair(certPath, keyPath); err == nil {
		if leaf, perr := x509.ParseCertificate(c.Certificate[0]); perr == nil && time.Until(leaf.NotAfter) > 30*24*time.Hour {
			return &c, nil
		}
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}
	serial, _ := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 120))
	host, _ := os.Hostname()
	tmpl := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: "SIP Santral", Organization: []string{"SIP Santral (self-signed)"}},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().AddDate(10, 0, 0),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		DNSNames:              []string{"localhost"},
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1")},
	}
	if host != "" {
		tmpl.DNSNames = append(tmpl.DNSNames, host)
	}
	if addrs, err := net.InterfaceAddrs(); err == nil {
		for _, a := range addrs {
			if ipn, ok := a.(*net.IPNet); ok && !ipn.IP.IsLoopback() {
				tmpl.IPAddresses = append(tmpl.IPAddresses, ipn.IP)
			}
		}
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		return nil, err
	}
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return nil, err
	}
	if err := writeFileAtomic(certPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o644); err != nil {
		return nil, err
	}
	if err := writeFileAtomic(keyPath, pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}), 0o600); err != nil {
		return nil, err
	}
	c, err := tls.LoadX509KeyPair(certPath, keyPath)
	if err != nil {
		return nil, err
	}
	return &c, nil
}
