// Package store manages TLS certificate generation, caching, and persistence.
package store

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"sync"
	"time"

	lru "github.com/hashicorp/golang-lru/v2"
	"github.com/rs/zerolog"
	"golang.org/x/sync/singleflight"

	"ffuuzz/internal/config"
	"ffuuzz/internal/metrics"
)

type CertStore struct {
	dir        string
	memOnly    bool
	root       *tls.Certificate
	rootX      *x509.Certificate
	rootKey    crypto.PrivateKey
	mu         sync.Mutex
	cache      *lru.Cache[string, *tls.Certificate]
	logger     zerolog.Logger
	tlsMinVer  uint16
	tlsTimeout time.Duration
	tlsCiphers []uint16
	tlsNoTick  bool
	sfGroup    singleflight.Group
}

func NewCertStore(cfg config.CertCacheConfig, tlsCfg config.TLSConfig, logger zerolog.Logger) (*CertStore, error) {
	if cfg.CertDir == "" && !cfg.MemoryOnly {
		return nil, errors.New("cert_dir required when memory_only is false")
	}

	if !cfg.MemoryOnly {
		if err := os.MkdirAll(cfg.CertDir, 0750); err != nil {
			return nil, fmt.Errorf("create cert dir: %w", err)
		}
	}

	maxEntries := cfg.MaxEntries
	if maxEntries <= 0 {
		maxEntries = 1000
	}

	cache, err := lru.NewWithEvict[string, *tls.Certificate](maxEntries, func(_ string, _ *tls.Certificate) {
		metrics.CertCacheEvictions.Inc()
	})
	if err != nil {
		return nil, fmt.Errorf("create LRU cache: %w", err)
	}

	cs := &CertStore{
		dir:        cfg.CertDir,
		memOnly:    cfg.MemoryOnly,
		cache:      cache,
		logger:     logger,
		tlsMinVer:  tlsCfg.MinVersion,
		tlsTimeout: tlsCfg.HandshakeTimeout,
		tlsCiphers: tlsCfg.CipherSuites,
		tlsNoTick:  tlsCfg.DisableSessionTickets,
	}

	if cs.tlsMinVer == 0 {
		cs.tlsMinVer = tls.VersionTLS12
	}
	if cs.tlsTimeout == 0 {
		cs.tlsTimeout = 10 * time.Second
	}

	if err := cs.loadOrCreateRoot(); err != nil {
		return nil, err
	}
	return cs, nil
}

// TLSConfigForClient returns a tls.Config suitable for the MITM client-facing side.
func (c *CertStore) TLSConfigForClient(leaf tls.Certificate) *tls.Config {
	cfg := &tls.Config{
		Certificates:           []tls.Certificate{leaf},
		MinVersion:             c.tlsMinVer,
		SessionTicketsDisabled: c.tlsNoTick,
	}
	if len(c.tlsCiphers) > 0 {
		cfg.CipherSuites = append([]uint16(nil), c.tlsCiphers...)
	}
	return cfg
}

// HandshakeTimeout returns the configured TLS handshake timeout.
func (c *CertStore) HandshakeTimeout() time.Duration {
	return c.tlsTimeout
}

func (c *CertStore) loadOrCreateRoot() error {
	if c.memOnly {
		return c.createRoot()
	}

	certPath := filepath.Join(c.dir, "ca.pem")
	keyPath := filepath.Join(c.dir, "ca.key")

	if _, err := os.Stat(certPath); err == nil {
		certPEM, err := os.ReadFile(certPath)
		if err != nil {
			return err
		}
		keyPEM, err := os.ReadFile(keyPath)
		if err != nil {
			return err
		}
		tlscert, err := tls.X509KeyPair(certPEM, keyPEM)
		if err != nil {
			return err
		}
		xs, err := x509.ParseCertificate(tlscert.Certificate[0])
		if err != nil {
			return err
		}
		c.root = &tlscert
		c.rootX = xs
		c.rootKey = tlscert.PrivateKey
		return nil
	}

	if err := c.createRoot(); err != nil {
		return err
	}

	// Persist to disk atomically
	certPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: c.root.Certificate[0],
	})
	keyBytes, err := x509.MarshalPKCS8PrivateKey(c.rootKey)
	if err != nil {
		return err
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "PRIVATE KEY",
		Bytes: keyBytes,
	})

	certPath = filepath.Join(c.dir, "ca.pem")
	keyPath = filepath.Join(c.dir, "ca.key")
	if err := atomicWrite(certPath, certPEM, 0644); err != nil {
		return err
	}
	if err := atomicWrite(keyPath, keyPEM, 0600); err != nil {
		return err
	}
	return nil
}

func (c *CertStore) createRoot() error {
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return err
	}
	serial, err := rand.Int(rand.Reader, big.NewInt(1<<62))
	if err != nil {
		return fmt.Errorf("generate serial number: %w", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			Organization: []string{"ffuuzz Root CA"},
			CommonName:   "ffuuzz Root CA",
		},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(10 * 365 * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
		IsCA:                  true,
		MaxPathLen:            1,
	}

	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &priv.PublicKey, priv)
	if err != nil {
		return err
	}

	xs, err := x509.ParseCertificate(der)
	if err != nil {
		return err
	}

	tlscert := tls.Certificate{
		Certificate: [][]byte{der},
		PrivateKey:  priv,
	}
	c.root = &tlscert
	c.rootX = xs
	c.rootKey = priv
	return nil
}

// GetCertFor returns a leaf certificate for the given hostname, using the LRU
// cache. On cache miss it generates a new certificate with retry.
// Uses singleflight to prevent concurrent generation of the same certificate.
func (c *CertStore) GetCertFor(host string) (tls.Certificate, error) {
	c.mu.Lock()
	if cert, ok := c.cache.Get(host); ok {
		metrics.CertCacheHits.Inc()
		c.mu.Unlock()
		return *cert, nil
	}
	c.mu.Unlock()

	result, err, _ := c.sfGroup.Do(host, func() (any, error) {
		// Double-check cache inside singleflight to handle race
		c.mu.Lock()
		if cert, ok := c.cache.Get(host); ok {
			c.mu.Unlock()
			return *cert, nil
		}
		c.mu.Unlock()

		metrics.CertCacheMisses.Inc()

		const maxRetries = 3
		var lastErr error
		for attempt := range maxRetries {
			cert, err := c.generateLeaf(host)
			if err != nil {
				lastErr = err
				metrics.CertErrors.Inc()
				c.logger.Warn().Err(err).Str("host", host).Int("attempt", attempt+1).Msg("cert generation failed")
				time.Sleep(10 * time.Millisecond)
				continue
			}
			c.mu.Lock()
			c.cache.Add(host, &cert)
			c.mu.Unlock()
			return cert, nil
		}
		return tls.Certificate{}, fmt.Errorf("cert generation failed after %d attempts: %w", maxRetries, lastErr)
	})

	if err != nil {
		return tls.Certificate{}, err
	}
	return result.(tls.Certificate), nil
}

func (c *CertStore) generateLeaf(host string) (tls.Certificate, error) {
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return tls.Certificate{}, err
	}
	serial, err := rand.Int(rand.Reader, big.NewInt(1<<62))
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("generate serial number: %w", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			CommonName: host,
		},
		NotBefore:   time.Now().Add(-time.Hour),
		NotAfter:    time.Now().Add(365 * 24 * time.Hour),
		KeyUsage:    x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:    []string{host},
		IPAddresses: []net.IP{},
	}

	der, err := x509.CreateCertificate(rand.Reader, tmpl, c.rootX, &priv.PublicKey, c.root.PrivateKey)
	if err != nil {
		return tls.Certificate{}, err
	}

	if !c.memOnly {
		certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
		keyPEM := pem.EncodeToMemory(&pem.Block{
			Type:  "RSA PRIVATE KEY",
			Bytes: x509.MarshalPKCS1PrivateKey(priv),
		})
		if err := atomicWrite(filepath.Join(c.dir, host+".pem"), certPEM, 0644); err != nil {
			c.logger.Warn().Err(err).Str("host", host).Msg("failed to write cert to disk")
			metrics.CertErrors.Inc()
			// Non-fatal: cert still works in memory
		}
		if err := atomicWrite(filepath.Join(c.dir, host+".key"), keyPEM, 0600); err != nil {
			c.logger.Warn().Err(err).Str("host", host).Msg("failed to write key to disk")
			metrics.CertErrors.Inc()
		}
	}

	return tls.Certificate{
		Certificate: [][]byte{der},
		PrivateKey:  priv,
	}, nil
}

// atomicWrite writes data to a unique temp file then renames it to path.
// Using a unique temp file name avoids race conditions under concurrent access.
func atomicWrite(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return err
	}
	if err := tmp.Chmod(perm); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return err
	}
	return os.Rename(tmpName, path)
}
