package httpsintercept

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"testing"
	"time"
)

// generateTestKeys generates a new RSA private key and self-signed certificate
func generateTestKeys() ([]byte, []byte, []byte, error) {
	// Generate a new RSA private key
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, nil, nil, err
	}

	// Create a self-signed certificate
	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			Organization: []string{"Test Org"},
		},
		NotBefore: time.Now(),
		NotAfter:  time.Now().Add(365 * 24 * time.Hour),
		KeyUsage:  x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{
			x509.ExtKeyUsageServerAuth,
		},
	}

	certBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &privateKey.PublicKey, privateKey)
	if err != nil {
		return nil, nil, nil, err
	}

	// Encode the private key and certificate to PEM format
	privKeyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(privateKey)})
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certBytes})

	return certPEM, privKeyPEM, certPEM, nil
}

// TestNew tests the New function
func TestNew(t *testing.T) {
	pubKey, privKey, rootCA, err := generateTestKeys()
	if err != nil {
		t.Fatalf("Failed to generate test keys: %v", err)
	}

	intercept, err := New(pubKey, privKey, "", rootCA)
	if err != nil {
		t.Fatalf("Failed to create Intercept: %v", err)
	}

	if intercept.publicKey == nil || intercept.privateKey == nil || intercept.rootCA == nil {
		t.Fatalf("Intercept object not initialized correctly")
	}
}

// TestSetDomain tests the SetDomain function
func TestSetDomain(t *testing.T) {
	pubKey, privKey, rootCA, err := generateTestKeys()
	if err != nil {
		t.Fatalf("Failed to generate test keys: %v", err)
	}

	intercept, err := New(pubKey, privKey, "", rootCA)
	if err != nil {
		t.Fatalf("Failed to create Intercept: %v", err)
	}

	domain := "example.com"
	intercept.SetDomain(domain)

	if intercept.domain != domain {
		t.Fatalf("Expected domain %s, got %s", domain, intercept.domain)
	}
}

// TestGetCertificate tests the GetCertificate function
func TestGetCertificate(t *testing.T) {
	pubKey, privKey, rootCA, err := generateTestKeys()
	if err != nil {
		t.Fatalf("Failed to generate test keys: %v", err)
	}

	intercept, err := New(pubKey, privKey, "", rootCA)
	if err != nil {
		t.Fatalf("Failed to create Intercept: %v", err)
	}

	domain := "example.com"
	cert := intercept.GetCertificate(domain)

	if cert == nil {
		t.Fatalf("Failed to get certificate for domain %s", domain)
	}
}

// TestCreateCertificate tests the CreateCertificate function
func TestCreateCertificate(t *testing.T) {
	pubKey, privKey, rootCA, err := generateTestKeys()
	if err != nil {
		t.Fatalf("Failed to generate test keys: %v", err)
	}

	intercept, err := New(pubKey, privKey, "", rootCA)
	if err != nil {
		t.Fatalf("Failed to create Intercept: %v", err)
	}

	domain := "example.com"
	err = intercept.CreateCertificate(domain)
	if err != nil {
		t.Fatalf("Failed to create certificate for domain %s: %v", domain, err)
	}

	cert := intercept.GetCertificate(domain)
	if cert == nil {
		t.Fatalf("Failed to get certificate for domain %s", domain)
	}
}
