package cert

import (
	"Gleip/backend/paths"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// CertificateManager handles all certificate and CA related operations
type CertificateManager struct {
	caCert         *x509.Certificate
	caPrivateKey   *rsa.PrivateKey
	certCache      map[string]*tls.Certificate
	certCacheMutex sync.RWMutex
	caPath         string
}

// NewCertificateManager creates a new certificate manager
func NewCertificateManager() *CertificateManager {
	cm := &CertificateManager{
		certCache: make(map[string]*tls.Certificate),
		caPath:    paths.GlobalPaths.CertificatesDir,
	}

	// Ensure the CA directory exists
	if err := cm.ensureCADirectory(); err != nil {
		fmt.Printf("Warning: Failed to create CA directory: %v\n", err)
	}

	return cm
}

// ensureCADirectory ensures the CA directory exists
func (cm *CertificateManager) ensureCADirectory() error {
	return os.MkdirAll(cm.caPath, 0700)
}

// LoadCA loads the CA certificate and private key
// It uses the embedded CA certificate and key from gleip_certificate.go
// and saves it to disk for Firefox and other purposes
func (cm *CertificateManager) LoadCA() error {
	// Ensure CA directory exists
	if err := cm.ensureCADirectory(); err != nil {
		return fmt.Errorf("failed to create CA directory: %v", err)
	}

	certPath := filepath.Join(cm.caPath, "gleip.cer")
	keyPath := filepath.Join(cm.caPath, "gleip.key")

	// Always use the embedded certificate and key
	// Save them to disk first
	if err := os.WriteFile(certPath, EmbeddedCACert, 0600); err != nil {
		return fmt.Errorf("failed to save CA certificate: %v", err)
	}
	if err := os.WriteFile(keyPath, EmbeddedCAKey, 0600); err != nil {
		return fmt.Errorf("failed to save CA private key: %v", err)
	}

	// Load the certificate
	certBlock, _ := pem.Decode(EmbeddedCACert)
	if certBlock == nil {
		return fmt.Errorf("failed to parse CA certificate PEM")
	}

	x509Cert, err := x509.ParseCertificate(certBlock.Bytes)
	if err != nil {
		return fmt.Errorf("failed to parse CA certificate: %v", err)
	}
	cm.caCert = x509Cert

	// Load the private key
	keyBlock, _ := pem.Decode(EmbeddedCAKey)
	if keyBlock == nil {
		return fmt.Errorf("failed to parse CA private key PEM")
	}

	// Try PKCS8 first
	key, err := x509.ParsePKCS8PrivateKey(keyBlock.Bytes)
	if err != nil {
		// If PKCS8 fails, try PKCS1
		key, err = x509.ParsePKCS1PrivateKey(keyBlock.Bytes)
		if err != nil {
			return fmt.Errorf("failed to parse CA private key: %v", err)
		}
	}

	// Convert to RSA private key
	rsaKey, ok := key.(*rsa.PrivateKey)
	if !ok {
		return fmt.Errorf("CA private key is not an RSA key")
	}
	cm.caPrivateKey = rsaKey

	return nil
}

// GetCAPath returns the path to the CA directory
func (cm *CertificateManager) GetCAPath() string {
	return cm.caPath
}

// GenerateCertificate generates a new certificate for the given host
func (cm *CertificateManager) GenerateCertificate(host string) (*tls.Certificate, error) {
	// Make hostname lowercase to avoid cache misses due to case differences
	host = strings.ToLower(host)

	// Check cache first
	cm.certCacheMutex.RLock()
	if cert, ok := cm.certCache[host]; ok {
		// Check if certificate is still valid with at least 1 hour left
		if cert.Leaf != nil && time.Now().Add(24*time.Hour).Before(cert.Leaf.NotAfter) {
			fmt.Printf("Using cached certificate for %s (expires: %s)\n", host, cert.Leaf.NotAfter.Format(time.RFC3339))
			cm.certCacheMutex.RUnlock()
			return cert, nil
		}
		// Certificate is about to expire, generate a new one
		fmt.Printf("Certificate for %s is expiring soon (expires: %s), generating new one\n",
			host, cert.Leaf.NotAfter.Format(time.RFC3339))
	} else {
		fmt.Printf("No certificate found for %s in cache, generating new one\n", host)
	}
	cm.certCacheMutex.RUnlock()

	// Generate new certificate
	serialNumber, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, fmt.Errorf("failed to generate serial number: %v", err)
	}

	// Create certificate template for a leaf certificate
	template := &x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			CommonName:         host,
			Organization:       []string{"Gleip Generated Certificate"},
			OrganizationalUnit: []string{"Secure Proxy"},
			Country:            []string{"US"},
		},
		// Include the Issuer from the CA certificate for better validation
		Issuer:                cm.caCert.Subject,
		NotBefore:             time.Now().Add(-1 * time.Hour), // Start 1 hour ago to avoid time skew issues
		NotAfter:              time.Now().AddDate(1, 0, 0),    // Valid for 1 year
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
		IsCA:                  false,
	}

	// Add host to Subject Alternative Names
	if ip := net.ParseIP(host); ip != nil {
		template.IPAddresses = append(template.IPAddresses, ip)
	} else {
		template.DNSNames = append(template.DNSNames, host)

		// Add www. subdomain if the host doesn't already have it
		if !strings.HasPrefix(host, "www.") {
			template.DNSNames = append(template.DNSNames, "www."+host)
		}
	}

	// Generate private key
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, fmt.Errorf("failed to generate private key: %v", err)
	}

	// Create certificate
	certDER, err := x509.CreateCertificate(rand.Reader, template, cm.caCert, &privateKey.PublicKey, cm.caPrivateKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create certificate: %v", err)
	}

	// Parse the certificate to get the x509.Certificate object
	leaf, err := x509.ParseCertificate(certDER)
	if err != nil {
		return nil, fmt.Errorf("failed to parse generated certificate: %v", err)
	}

	// Encode certificate and private key
	certPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: certDER,
	})
	keyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(privateKey),
	})

	// Create tls.Certificate
	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return nil, fmt.Errorf("failed to create X509 key pair: %v", err)
	}

	// Attach the parsed certificate for easier access
	cert.Leaf = leaf

	// Cache the certificate
	cm.certCacheMutex.Lock()
	cm.certCache[host] = &cert
	cm.certCacheMutex.Unlock()

	return &cert, nil
}

// GetCertificateForConn implements the tls.GetCertificate callback for TLS servers
func (cm *CertificateManager) GetCertificateForConn(info *tls.ClientHelloInfo) (*tls.Certificate, error) {
	if info.ServerName == "" {
		// Use a default certificate if SNI is not provided
		return cm.GenerateCertificate("localhost")
	}

	return cm.GenerateCertificate(info.ServerName)
}

// ExportCertificate exports the generated certificate for a host to a file
func (cm *CertificateManager) ExportCertificate(host, path string) error {
	// Generate or get the certificate
	cert, err := cm.GenerateCertificate(host)
	if err != nil {
		return fmt.Errorf("failed to generate certificate: %v", err)
	}

	// Write the certificate to the file
	certPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: cert.Certificate[0],
	})

	if err := os.WriteFile(path, certPEM, 0644); err != nil {
		return fmt.Errorf("failed to write certificate to file: %v", err)
	}

	return nil
}

// GetCACertificatePath returns the path to the CA certificate file
func (cm *CertificateManager) GetCACertificatePath() string {
	return filepath.Join(cm.caPath, "gleip.cer")
}

// ClearCertificateCache clears the certificate cache
func (cm *CertificateManager) ClearCertificateCache() {
	cm.certCacheMutex.Lock()
	cm.certCache = make(map[string]*tls.Certificate)
	cm.certCacheMutex.Unlock()
}
