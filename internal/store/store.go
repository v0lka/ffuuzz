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
	"math/big"
	"net"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type CertStore struct {
	dir     string
	root    *tls.Certificate
	rootX   *x509.Certificate
	rootKey crypto.PrivateKey
	mu      sync.Mutex
	cache   map[string]*tls.Certificate
}

func NewCertStore(cadir string) (*CertStore, error) {
	if cadir == "" {
		return nil, errors.New("cadir required")
	}
	if err := os.MkdirAll(cadir, 0755); err != nil {
		return nil, err
	}
	cs := &CertStore{
		dir:   cadir,
		cache: make(map[string]*tls.Certificate),
	}
	if err := cs.loadOrCreateRoot(); err != nil {
		return nil, err
	}
	return cs, nil
}

// загрузка существующего CA или создание нового
func (c *CertStore) loadOrCreateRoot() error {
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

	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return err
	}
	serial, _ := rand.Int(rand.Reader, big.NewInt(1<<62))
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
		MaxPathLenZero:        false,
		MaxPathLen:            1,
	}

	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &priv.PublicKey, priv)
	if err != nil {
		return err
	}

	certPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: der,
	})
	keyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(priv),
	})
	if err := os.WriteFile(certPath, certPEM, 0644); err != nil {
		return err
	}
	if err := os.WriteFile(keyPath, keyPEM, 0600); err != nil {
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

// возвращает кэшированный leaf-сертификат для хоста
func (c *CertStore) GetCertFor(host string) (tls.Certificate, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if cert, ok := c.cache[host]; ok {
		return *cert, nil
	}

	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return tls.Certificate{}, err
	}
	serial, _ := rand.Int(rand.Reader, big.NewInt(1<<62))
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

	certPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: der,
	})
	keyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(priv),
	})

	_ = os.WriteFile(filepath.Join(c.dir, host+".pem"), certPEM, 0644)
	_ = os.WriteFile(filepath.Join(c.dir, host+".key"), keyPEM, 0600)

	tlscert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return tls.Certificate{}, err
	}
	c.cache[host] = &tlscert
	return tlscert, nil
}
