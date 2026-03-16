package store

import (
	"crypto/tls"
	"crypto/x509"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/rs/zerolog"

	"ffuuzz/internal/config"
)

func memoryOnlyCertStore(t *testing.T) *CertStore {
	t.Helper()
	cfg := config.CertCacheConfig{
		MaxEntries: 100,
		MemoryOnly: true,
	}
	tlsCfg := config.TLSConfig{
		MinVersion:       tls.VersionTLS12,
		HandshakeTimeout: 5 * time.Second,
	}
	logger := zerolog.Nop()
	cs, err := NewCertStore(cfg, tlsCfg, logger)
	if err != nil {
		t.Fatalf("NewCertStore: %v", err)
	}
	return cs
}

func TestNewCertStore_MemoryOnly(t *testing.T) {
	cs := memoryOnlyCertStore(t)
	if cs == nil {
		t.Fatal("expected non-nil CertStore")
	}
	if cs.root == nil {
		t.Error("root cert should be created")
	}
	if cs.rootX == nil {
		t.Error("rootX should be set")
	}
	if cs.rootKey == nil {
		t.Error("rootKey should be set")
	}
}

func TestNewCertStore_DiskMode(t *testing.T) {
	dir := t.TempDir()
	cfg := config.CertCacheConfig{
		MaxEntries: 50,
		CertDir:    dir,
	}
	tlsCfg := config.TLSConfig{}
	cs, err := NewCertStore(cfg, tlsCfg, zerolog.Nop())
	if err != nil {
		t.Fatalf("NewCertStore: %v", err)
	}
	if cs == nil {
		t.Fatal("expected non-nil CertStore")
	}
}

func TestNewCertStore_NoDirNotMemoryOnly(t *testing.T) {
	cfg := config.CertCacheConfig{
		MaxEntries: 50,
		MemoryOnly: false,
		CertDir:    "",
	}
	_, err := NewCertStore(cfg, config.TLSConfig{}, zerolog.Nop())
	if err == nil {
		t.Fatal("expected error with no cert_dir and not memory_only")
	}
}

func TestNewCertStore_DefaultEntries(t *testing.T) {
	cfg := config.CertCacheConfig{
		MaxEntries: 0, // should default to 1000
		MemoryOnly: true,
	}
	cs, err := NewCertStore(cfg, config.TLSConfig{}, zerolog.Nop())
	if err != nil {
		t.Fatalf("NewCertStore: %v", err)
	}
	if cs == nil {
		t.Fatal("expected non-nil CertStore")
	}
}

func TestGetCertFor_Basic(t *testing.T) {
	cs := memoryOnlyCertStore(t)
	cert, err := cs.GetCertFor("example.com")
	if err != nil {
		t.Fatalf("GetCertFor: %v", err)
	}
	if len(cert.Certificate) == 0 {
		t.Error("expected at least one certificate")
	}
	if cert.PrivateKey == nil {
		t.Error("expected private key")
	}
}

func TestGetCertFor_Cached(t *testing.T) {
	cs := memoryOnlyCertStore(t)
	cert1, err := cs.GetCertFor("cached.example.com")
	if err != nil {
		t.Fatalf("first GetCertFor: %v", err)
	}
	cert2, err := cs.GetCertFor("cached.example.com")
	if err != nil {
		t.Fatalf("second GetCertFor: %v", err)
	}
	// Same cert should be returned from cache
	if len(cert1.Certificate[0]) != len(cert2.Certificate[0]) {
		t.Error("cached cert should be identical")
	}
}

func TestGetCertFor_DifferentHosts(t *testing.T) {
	cs := memoryOnlyCertStore(t)
	cert1, err := cs.GetCertFor("a.example.com")
	if err != nil {
		t.Fatalf("GetCertFor a: %v", err)
	}
	cert2, err := cs.GetCertFor("b.example.com")
	if err != nil {
		t.Fatalf("GetCertFor b: %v", err)
	}
	// Different hosts should produce different certs
	if string(cert1.Certificate[0]) == string(cert2.Certificate[0]) {
		t.Error("expected different certs for different hosts")
	}
}

func TestTLSConfigForClient(t *testing.T) {
	cs := memoryOnlyCertStore(t)
	cert, _ := cs.GetCertFor("test.example.com")
	cfg := cs.TLSConfigForClient(cert)
	if cfg == nil {
		t.Fatal("expected non-nil TLS config")
	}
	if cfg.MinVersion != tls.VersionTLS12 {
		t.Errorf("MinVersion = %d, want TLS 1.2", cfg.MinVersion)
	}
	if len(cfg.Certificates) != 1 {
		t.Errorf("expected 1 certificate in TLS config")
	}
}

func TestTLSConfigForClient_SessionTicketsDisabled(t *testing.T) {
	cfg := config.CertCacheConfig{
		MaxEntries: 10,
		MemoryOnly: true,
	}
	tlsCfg := config.TLSConfig{
		DisableSessionTickets: true,
	}
	cs, err := NewCertStore(cfg, tlsCfg, zerolog.Nop())
	if err != nil {
		t.Fatalf("NewCertStore: %v", err)
	}
	cert, _ := cs.GetCertFor("test.com")
	c := cs.TLSConfigForClient(cert)
	if !c.SessionTicketsDisabled {
		t.Error("expected SessionTicketsDisabled=true")
	}
}

func TestTLSConfigForClient_CipherSuites(t *testing.T) {
	ciphers := []uint16{tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256}
	cfg := config.CertCacheConfig{
		MaxEntries: 10,
		MemoryOnly: true,
	}
	tlsCfg := config.TLSConfig{
		CipherSuites: ciphers,
	}
	cs, err := NewCertStore(cfg, tlsCfg, zerolog.Nop())
	if err != nil {
		t.Fatalf("NewCertStore: %v", err)
	}
	cert, _ := cs.GetCertFor("test.com")
	c := cs.TLSConfigForClient(cert)
	if len(c.CipherSuites) != 1 {
		t.Fatalf("expected 1 cipher suite, got %d", len(c.CipherSuites))
	}
}

func TestHandshakeTimeout(t *testing.T) {
	cs := memoryOnlyCertStore(t)
	if cs.HandshakeTimeout() != 5*time.Second {
		t.Errorf("HandshakeTimeout = %v, want 5s", cs.HandshakeTimeout())
	}
}

func TestHandshakeTimeout_Default(t *testing.T) {
	cfg := config.CertCacheConfig{MemoryOnly: true}
	tlsCfg := config.TLSConfig{} // HandshakeTimeout == 0
	cs, _ := NewCertStore(cfg, tlsCfg, zerolog.Nop())
	if cs.HandshakeTimeout() != 10*time.Second {
		t.Errorf("HandshakeTimeout = %v, want default 10s", cs.HandshakeTimeout())
	}
}

func TestNewCertStore_DiskPersistence(t *testing.T) {
	dir := t.TempDir()
	cfg := config.CertCacheConfig{
		MaxEntries: 50,
		CertDir:    dir,
	}
	tlsCfg := config.TLSConfig{}

	// First creation generates root CA
	cs1, err := NewCertStore(cfg, tlsCfg, zerolog.Nop())
	if err != nil {
		t.Fatalf("first NewCertStore: %v", err)
	}

	// Generate a leaf cert
	_, err = cs1.GetCertFor("persist.example.com")
	if err != nil {
		t.Fatalf("GetCertFor: %v", err)
	}

	// Second creation should load existing root CA from disk
	cs2, err := NewCertStore(cfg, tlsCfg, zerolog.Nop())
	if err != nil {
		t.Fatalf("second NewCertStore: %v", err)
	}
	if cs2.root == nil {
		t.Error("expected root to be loaded from disk")
	}
}

func TestAtomicWrite(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/test.txt"
	data := []byte("test data")
	if err := atomicWrite(zerolog.Nop(), path, data, 0644); err != nil {
		t.Fatalf("atomicWrite: %v", err)
	}
	// Verify file contents
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(got) != "test data" {
		t.Errorf("file contents = %q, want 'test data'", string(got))
	}
}

func TestAtomicWrite_Permissions(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/secret.key"
	if err := atomicWrite(zerolog.Nop(), path, []byte("secret"), 0600); err != nil {
		t.Fatalf("atomicWrite: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	perm := info.Mode().Perm()
	if perm != 0600 {
		t.Errorf("permissions = %o, want 0600", perm)
	}
}

func TestAtomicWrite_Overwrite(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/overwrite.txt"
	if err := atomicWrite(zerolog.Nop(), path, []byte("first"), 0644); err != nil {
		t.Fatalf("first atomicWrite: %v", err)
	}
	if err := atomicWrite(zerolog.Nop(), path, []byte("second"), 0644); err != nil {
		t.Fatalf("second atomicWrite: %v", err)
	}
	got, _ := os.ReadFile(path)
	if string(got) != "second" {
		t.Errorf("file contents = %q, want 'second'", string(got))
	}
}

func TestGetCertFor_DiskMode(t *testing.T) {
	dir := t.TempDir()
	cfg := config.CertCacheConfig{
		MaxEntries: 50,
		CertDir:    dir,
	}
	cs, err := NewCertStore(cfg, config.TLSConfig{}, zerolog.Nop())
	if err != nil {
		t.Fatalf("NewCertStore: %v", err)
	}

	cert, err := cs.GetCertFor("disk.example.com")
	if err != nil {
		t.Fatalf("GetCertFor: %v", err)
	}
	if len(cert.Certificate) == 0 {
		t.Error("expected certificate")
	}

	// Verify cert and key files were written to disk
	certPath := filepath.Join(dir, "disk.example.com.pem")
	keyPath := filepath.Join(dir, "disk.example.com.key")
	if _, err := os.Stat(certPath); err != nil {
		t.Errorf("cert file not found: %v", err)
	}
	if _, err := os.Stat(keyPath); err != nil {
		t.Errorf("key file not found: %v", err)
	}
}

func TestGetCertFor_CertChainValid(t *testing.T) {
	cs := memoryOnlyCertStore(t)
	cert, err := cs.GetCertFor("valid.example.com")
	if err != nil {
		t.Fatalf("GetCertFor: %v", err)
	}

	// Parse the leaf cert and verify it was signed by the root
	leaf, err := x509.ParseCertificate(cert.Certificate[0])
	if err != nil {
		t.Fatalf("ParseCertificate: %v", err)
	}

	if leaf.Subject.CommonName != "valid.example.com" {
		t.Errorf("CN = %q, want valid.example.com", leaf.Subject.CommonName)
	}

	// Verify the cert chain
	roots := x509.NewCertPool()
	roots.AddCert(cs.rootX)
	_, err = leaf.Verify(x509.VerifyOptions{
		Roots: roots,
	})
	if err != nil {
		t.Errorf("cert chain verification failed: %v", err)
	}
}

func TestGetCertFor_LeafHasDNSNames(t *testing.T) {
	cs := memoryOnlyCertStore(t)
	cert, err := cs.GetCertFor("dns.example.com")
	if err != nil {
		t.Fatalf("GetCertFor: %v", err)
	}
	leaf, _ := x509.ParseCertificate(cert.Certificate[0])
	found := false
	for _, name := range leaf.DNSNames {
		if name == "dns.example.com" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected DNS name 'dns.example.com' in cert, got %v", leaf.DNSNames)
	}
}

func TestCreateRoot_ProducesCA(t *testing.T) {
	cs := memoryOnlyCertStore(t)
	if !cs.rootX.IsCA {
		t.Error("root cert should be a CA")
	}
	if cs.rootX.Subject.CommonName != "ffuuzz Root CA" {
		t.Errorf("root CN = %q, want 'ffuuzz Root CA'", cs.rootX.Subject.CommonName)
	}
}

func TestNewCertStore_DiskMode_ReloadRootCA(t *testing.T) {
	dir := t.TempDir()
	cfg := config.CertCacheConfig{
		MaxEntries: 50,
		CertDir:    dir,
	}

	// First store creates root CA
	cs1, err := NewCertStore(cfg, config.TLSConfig{}, zerolog.Nop())
	if err != nil {
		t.Fatalf("first NewCertStore: %v", err)
	}
	originalRoot := cs1.rootX.SerialNumber

	// Second store loads root CA from disk
	cs2, err := NewCertStore(cfg, config.TLSConfig{}, zerolog.Nop())
	if err != nil {
		t.Fatalf("second NewCertStore: %v", err)
	}

	if cs2.rootX.SerialNumber.Cmp(originalRoot) != 0 {
		t.Error("reloaded root CA serial number doesn't match original")
	}
}

func TestNewCertStore_TLSMinVersionDefault(t *testing.T) {
	cfg := config.CertCacheConfig{MemoryOnly: true}
	cs, err := NewCertStore(cfg, config.TLSConfig{}, zerolog.Nop())
	if err != nil {
		t.Fatalf("NewCertStore: %v", err)
	}
	if cs.tlsMinVer != tls.VersionTLS12 {
		t.Errorf("default tlsMinVer = %d, want TLS 1.2 (%d)", cs.tlsMinVer, tls.VersionTLS12)
	}
}

func TestNewCertStore_TLSMinVersionCustom(t *testing.T) {
	cfg := config.CertCacheConfig{MemoryOnly: true}
	tlsCfg := config.TLSConfig{MinVersion: tls.VersionTLS13}
	cs, err := NewCertStore(cfg, tlsCfg, zerolog.Nop())
	if err != nil {
		t.Fatalf("NewCertStore: %v", err)
	}
	if cs.tlsMinVer != tls.VersionTLS13 {
		t.Errorf("custom tlsMinVer = %d, want TLS 1.3 (%d)", cs.tlsMinVer, tls.VersionTLS13)
	}
}

func TestNewCertStore_BadDir(t *testing.T) {
	cfg := config.CertCacheConfig{
		MaxEntries: 50,
		CertDir:    "/dev/null/nonexistent", // unwritable
	}
	_, err := NewCertStore(cfg, config.TLSConfig{}, zerolog.Nop())
	if err == nil {
		t.Fatal("expected error with bad cert dir")
	}
}

func TestGetCertFor_CacheEviction(t *testing.T) {
	cfg := config.CertCacheConfig{
		MaxEntries: 2, // very small cache
		MemoryOnly: true,
	}
	cs, err := NewCertStore(cfg, config.TLSConfig{}, zerolog.Nop())
	if err != nil {
		t.Fatalf("NewCertStore: %v", err)
	}

	// Fill cache
	_, err = cs.GetCertFor("a.example.com")
	if err != nil {
		t.Fatalf("GetCertFor a: %v", err)
	}
	_, err = cs.GetCertFor("b.example.com")
	if err != nil {
		t.Fatalf("GetCertFor b: %v", err)
	}
	// This should evict "a"
	_, err = cs.GetCertFor("c.example.com")
	if err != nil {
		t.Fatalf("GetCertFor c: %v", err)
	}

	// "a" should now be regenerated (not cached)
	_, err = cs.GetCertFor("a.example.com")
	if err != nil {
		t.Fatalf("GetCertFor a again: %v", err)
	}
}

func TestGetCertFor_DiskMode_WritesCertAndKey(t *testing.T) {
	dir := t.TempDir()
	cfg := config.CertCacheConfig{
		MaxEntries: 50,
		CertDir:    dir,
	}
	cs, err := NewCertStore(cfg, config.TLSConfig{}, zerolog.Nop())
	if err != nil {
		t.Fatalf("NewCertStore: %v", err)
	}

	_, err = cs.GetCertFor("writetest.example.com")
	if err != nil {
		t.Fatalf("GetCertFor: %v", err)
	}

	// Verify both cert and key files exist
	certFile := filepath.Join(dir, "writetest.example.com.pem")
	keyFile := filepath.Join(dir, "writetest.example.com.key")

	certData, err := os.ReadFile(certFile)
	if err != nil {
		t.Fatalf("read cert: %v", err)
	}
	if len(certData) == 0 {
		t.Error("cert file is empty")
	}

	keyData, err := os.ReadFile(keyFile)
	if err != nil {
		t.Fatalf("read key: %v", err)
	}
	if len(keyData) == 0 {
		t.Error("key file is empty")
	}

	// Key file should have restricted permissions
	info, _ := os.Stat(keyFile)
	perm := info.Mode().Perm()
	if perm != 0600 {
		t.Errorf("key permissions = %o, want 0600", perm)
	}
}

func TestLoadOrCreateRoot_CorruptCert(t *testing.T) {
	dir := t.TempDir()

	// Write corrupt cert and key files
	_ = os.WriteFile(filepath.Join(dir, "ca.pem"), []byte("not a cert"), 0644)
	_ = os.WriteFile(filepath.Join(dir, "ca.key"), []byte("not a key"), 0600)

	cfg := config.CertCacheConfig{
		MaxEntries: 50,
		CertDir:    dir,
	}
	_, err := NewCertStore(cfg, config.TLSConfig{}, zerolog.Nop())
	if err == nil {
		t.Fatal("expected error with corrupt cert files")
	}
}

func TestLoadOrCreateRoot_MissingKeyFile(t *testing.T) {
	dir := t.TempDir()

	// Create a valid cert file but no key file
	cfg1 := config.CertCacheConfig{MaxEntries: 50, CertDir: dir}
	cs1, _ := NewCertStore(cfg1, config.TLSConfig{}, zerolog.Nop())
	_ = cs1

	// Remove the key file
	_ = os.Remove(filepath.Join(dir, "ca.key"))

	// Try to load again - should fail because cert exists but key doesn't
	_, err := NewCertStore(cfg1, config.TLSConfig{}, zerolog.Nop())
	if err == nil {
		t.Fatal("expected error with missing key file")
	}
}

func TestLoadOrCreateRoot_UnreadableCertFile(t *testing.T) {
	dir := t.TempDir()

	// Create a valid store first to generate the CA files
	cfg := config.CertCacheConfig{MaxEntries: 50, CertDir: dir}
	_, err := NewCertStore(cfg, config.TLSConfig{}, zerolog.Nop())
	if err != nil {
		t.Fatalf("first NewCertStore: %v", err)
	}

	// Make the cert file unreadable
	certPath := filepath.Join(dir, "ca.pem")
	_ = os.Chmod(certPath, 0000)
	defer func() { _ = os.Chmod(certPath, 0644) }()

	// Try to load again - should fail because cert exists but is unreadable
	_, err = NewCertStore(cfg, config.TLSConfig{}, zerolog.Nop())
	if err == nil {
		t.Fatal("expected error with unreadable cert file")
	}
}

func TestAtomicWrite_BadParentDir(t *testing.T) {
	err := atomicWrite(zerolog.Nop(), "/dev/null/cannot/write/here.txt", []byte("data"), 0644)
	if err == nil {
		t.Fatal("expected error writing to non-existent directory")
	}
}

func TestGenerateLeaf_DiskMode_MultipleCerts(t *testing.T) {
	dir := t.TempDir()
	cfg := config.CertCacheConfig{
		MaxEntries: 50,
		CertDir:    dir,
	}
	cs, err := NewCertStore(cfg, config.TLSConfig{}, zerolog.Nop())
	if err != nil {
		t.Fatalf("NewCertStore: %v", err)
	}

	hosts := []string{"host1.com", "host2.com", "host3.com"}
	for _, h := range hosts {
		_, err := cs.GetCertFor(h)
		if err != nil {
			t.Fatalf("GetCertFor(%s): %v", h, err)
		}
	}

	// Verify all cert files exist
	for _, h := range hosts {
		certFile := filepath.Join(dir, h+".pem")
		if _, err := os.Stat(certFile); err != nil {
			t.Errorf("cert file for %s not found: %v", h, err)
		}
	}
}

func TestGenerateLeaf_DiskWriteError(t *testing.T) {
	dir := t.TempDir()
	cfg := config.CertCacheConfig{
		MaxEntries: 50,
		CertDir:    dir,
	}
	cs, err := NewCertStore(cfg, config.TLSConfig{}, zerolog.Nop())
	if err != nil {
		t.Fatalf("NewCertStore: %v", err)
	}

	// Make the directory read-only so atomicWrite will fail for leaf certs
	_ = os.Chmod(dir, 0555)
	defer func() { _ = os.Chmod(dir, 0755) }() // restore for cleanup

	// GetCertFor should still succeed (disk write failure is non-fatal)
	cert, err := cs.GetCertFor("diskfail.example.com")
	if err != nil {
		t.Fatalf("GetCertFor should succeed even with disk errors: %v", err)
	}
	if len(cert.Certificate) == 0 {
		t.Error("expected certificate despite disk write failure")
	}
}

func TestGetCertFor_IPAddress(t *testing.T) {
	cs := memoryOnlyCertStore(t)
	cert, err := cs.GetCertFor("192.168.1.1")
	if err != nil {
		t.Fatalf("GetCertFor IP: %v", err)
	}
	if len(cert.Certificate) == 0 {
		t.Error("expected certificate for IP address")
	}
	leaf, _ := x509.ParseCertificate(cert.Certificate[0])
	if leaf.Subject.CommonName != "192.168.1.1" {
		t.Errorf("CN = %q, want 192.168.1.1", leaf.Subject.CommonName)
	}
}
