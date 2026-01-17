//nolint:revive,gocritic,errname,unconvert
package tls

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"log/slog"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"sync"
	"time"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/enterprise/cli/boundary/config"
)

type Manager interface {
	SetupTLSAndWriteCACert() (*tls.Config, string, string, error)
}

type Config struct {
	Logger    *slog.Logger
	ConfigDir string
	Uid       int
	Gid       int
}

// CertificateManager manages TLS certificates for the proxy
type CertificateManager struct {
	caKey     *rsa.PrivateKey
	caCert    *x509.Certificate
	certCache map[string]*tls.Certificate
	mutex     sync.RWMutex
	logger    *slog.Logger
	configDir string
	uid       int
	gid       int
}

// NewCertificateManager creates a new certificate manager
func NewCertificateManager(config Config) (*CertificateManager, error) {
	cm := &CertificateManager{
		certCache: make(map[string]*tls.Certificate),
		logger:    config.Logger,
		configDir: config.ConfigDir,
		uid:       config.Uid,
		gid:       config.Gid,
	}

	// Load or generate CA certificate
	err := cm.loadOrGenerateCA()
	if err != nil {
		return nil, xerrors.Errorf("failed to load or generate CA: %v", err)
	}

	return cm, nil
}

// SetupTLSAndWriteCACert sets up TLS config and writes CA certificate to file
// Returns the TLS config, CA cert path, and config directory
func (cm *CertificateManager) SetupTLSAndWriteCACert() (*tls.Config, error) {
	// Get TLS config
	tlsConfig := cm.getTLSConfig()

	// Get CA certificate PEM
	caCertPEM, err := cm.getCACertPEM()
	if err != nil {
		return nil, xerrors.Errorf("failed to get CA certificate: %v", err)
	}

	// Write CA certificate to file
	caCertPath := filepath.Join(cm.configDir, config.CACertName)
	err = os.WriteFile(caCertPath, caCertPEM, 0o600)
	if err != nil {
		return nil, xerrors.Errorf("failed to write CA certificate file: %v", err)
	}

	return tlsConfig, nil
}

// loadOrGenerateCA loads existing CA or generates a new one
func (cm *CertificateManager) loadOrGenerateCA() error {
	caKeyPath := filepath.Join(cm.configDir, config.CAKeyName)
	caCertPath := filepath.Join(cm.configDir, config.CACertName)

	cm.logger.Debug("paths", "cm.configDir", cm.configDir, "caCertPath", caCertPath)

	// Try to load existing CA
	if cm.loadExistingCA(caKeyPath, caCertPath) {
		cm.logger.Debug("Loaded existing CA certificate")
		return nil
	}

	// Generate new CA
	cm.logger.Info("Generating new CA certificate")
	return cm.generateCA(caKeyPath, caCertPath)
}

// getTLSConfig returns a TLS config that generates certificates on-demand
func (cm *CertificateManager) getTLSConfig() *tls.Config {
	return &tls.Config{
		GetCertificate: cm.getCertificate,
		MinVersion:     tls.VersionTLS12,
	}
}

// getCACertPEM returns the CA certificate in PEM format
func (cm *CertificateManager) getCACertPEM() ([]byte, error) {
	return pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: cm.caCert.Raw,
	}), nil
}

// loadExistingCA attempts to load existing CA files
func (cm *CertificateManager) loadExistingCA(keyPath, certPath string) bool {
	// Check if files exist
	if _, err := os.Stat(keyPath); os.IsNotExist(err) {
		return false
	}
	if _, err := os.Stat(certPath); os.IsNotExist(err) {
		return false
	}

	// Load private key
	keyData, err := os.ReadFile(keyPath)
	if err != nil {
		cm.logger.Warn("Failed to read CA key", "error", err)
		return false
	}

	keyBlock, _ := pem.Decode(keyData)
	if keyBlock == nil {
		cm.logger.Warn("Failed to decode CA key PEM")
		return false
	}

	privateKey, err := x509.ParsePKCS1PrivateKey(keyBlock.Bytes)
	if err != nil {
		cm.logger.Warn("Failed to parse CA private key", "error", err)
		return false
	}

	// Load certificate
	certData, err := os.ReadFile(certPath)
	if err != nil {
		cm.logger.Warn("Failed to read CA cert", "error", err)
		return false
	}

	certBlock, _ := pem.Decode(certData)
	if certBlock == nil {
		cm.logger.Warn("Failed to decode CA cert PEM")
		return false
	}

	cert, err := x509.ParseCertificate(certBlock.Bytes)
	if err != nil {
		cm.logger.Warn("Failed to parse CA certificate", "error", err)
		return false
	}

	// Check if certificate is still valid
	if time.Now().After(cert.NotAfter) {
		cm.logger.Warn("CA certificate has expired")
		return false
	}

	cm.caKey = privateKey
	cm.caCert = cert
	return true
}

// generateCA generates a new CA certificate and key
func (cm *CertificateManager) generateCA(keyPath, certPath string) error {
	// Create config directory if it doesn't exist
	err := os.MkdirAll(cm.configDir, 0o700)
	if err != nil {
		return xerrors.Errorf("failed to create config directory at %s: %v", cm.configDir, err)
	}

	// ensure the directory is owned by the original user
	err = os.Chown(cm.configDir, cm.uid, cm.gid)
	if err != nil {
		cm.logger.Warn("Failed to change config directory ownership", "error", err)
	}

	// Generate private key
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return xerrors.Errorf("failed to generate private key: %v", err)
	}

	// Create certificate template
	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			Organization:  []string{"coder"},
			Country:       []string{"US"},
			Province:      []string{""},
			Locality:      []string{""},
			StreetAddress: []string{""},
			PostalCode:    []string{""},
			CommonName:    "coder CA",
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(365 * 24 * time.Hour), // 1 year
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IsCA:                  true,
	}

	// Create certificate
	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &privateKey.PublicKey, privateKey)
	if err != nil {
		return xerrors.Errorf("failed to create certificate: %v", err)
	}

	// Parse certificate
	cert, err := x509.ParseCertificate(certDER)
	if err != nil {
		return xerrors.Errorf("failed to parse certificate: %v", err)
	}

	// Save private key
	keyFile, err := os.OpenFile(keyPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return xerrors.Errorf("failed to create key file: %v", err)
	}
	defer func() {
		err := keyFile.Close()
		if err != nil {
			cm.logger.Error("Failed to close key file", "error", err)
		}
	}()

	err = pem.Encode(keyFile, &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(privateKey),
	})
	if err != nil {
		return xerrors.Errorf("failed to write key to file: %v", err)
	}

	// Save certificate
	certFile, err := os.OpenFile(certPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return xerrors.Errorf("failed to create cert file: %v", err)
	}
	defer func() {
		err := certFile.Close()
		if err != nil {
			cm.logger.Error("Failed to close cert file", "error", err)
		}
	}()

	err = pem.Encode(certFile, &pem.Block{
		Type:  "CERTIFICATE",
		Bytes: certDER,
	})
	if err != nil {
		return xerrors.Errorf("failed to write cert to file: %v", err)
	}

	cm.caKey = privateKey
	cm.caCert = cert

	return nil
}

// getCertificate generates or retrieves a certificate for the given hostname
func (cm *CertificateManager) getCertificate(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
	hostname := hello.ServerName
	if hostname == "" {
		return nil, xerrors.New("no server name provided")
	}

	// Check cache first
	cm.mutex.RLock()
	if cert, exists := cm.certCache[hostname]; exists {
		cm.mutex.RUnlock()
		return cert, nil
	}
	cm.mutex.RUnlock()

	// Generate new certificate
	cm.mutex.Lock()
	defer cm.mutex.Unlock()

	// Double-check cache (another goroutine might have generated it)
	if cert, exists := cm.certCache[hostname]; exists {
		return cert, nil
	}

	cert, err := cm.generateServerCertificate(hostname)
	if err != nil {
		return nil, xerrors.Errorf("failed to generate certificate for %s: %v", hostname, err)
	}

	cm.certCache[hostname] = cert
	cm.logger.Debug("Generated certificate", "hostname", hostname)

	return cert, nil
}

// generateServerCertificate generates a server certificate for the given hostname
func (cm *CertificateManager) generateServerCertificate(hostname string) (*tls.Certificate, error) {
	// Generate private key
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, xerrors.Errorf("failed to generate private key: %v", err)
	}

	// Create certificate template
	template := x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano()),
		Subject: pkix.Name{
			Organization:  []string{"coder"},
			Country:       []string{"US"},
			Province:      []string{""},
			Locality:      []string{""},
			StreetAddress: []string{""},
			PostalCode:    []string{""},
			CommonName:    hostname,
		},
		NotBefore:   time.Now(),
		NotAfter:    time.Now().Add(24 * time.Hour), // 1 day
		KeyUsage:    x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:    []string{hostname},
	}

	// Add IP address if hostname is an IP
	if ip := net.ParseIP(hostname); ip != nil {
		template.IPAddresses = []net.IP{ip}
	}

	// Create certificate
	certDER, err := x509.CreateCertificate(rand.Reader, &template, cm.caCert, &privateKey.PublicKey, cm.caKey)
	if err != nil {
		return nil, xerrors.Errorf("failed to create certificate: %v", err)
	}

	// Create TLS certificate
	tlsCert := &tls.Certificate{
		Certificate: [][]byte{certDER},
		PrivateKey:  privateKey,
	}

	cm.logger.Debug("Generated certificate", "hostname", hostname)

	return tlsCert, nil
}
