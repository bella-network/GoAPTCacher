package httpsintercept

import (
	"crypto/ecdsa"
	"crypto/elliptic"
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

// TestSetAIAAddress tests the SetAIAAddress function
func TestSetAIAAddress(t *testing.T) {
	pubKey, privKey, rootCA, err := generateTestKeys()
	if err != nil {
		t.Fatalf("Failed to generate test keys: %v", err)
	}

	intercept, err := New(pubKey, privKey, "", rootCA)
	if err != nil {
		t.Fatalf("Failed to create Intercept: %v", err)
	}

	domain := "example.com"
	intercept.SetAIAAddress(domain)

	if intercept.aiaAddress != domain {
		t.Fatalf("Expected domain %s, got %s", domain, intercept.aiaAddress)
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

// generateTestECDSAKeys generates a new ECDSA private key and self-signed certificate
func generateTestECDSAKeys() ([]byte, []byte, []byte, error) {
	// Generate a new ECDSA private key
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
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
	privKeyPEM, err := x509.MarshalECPrivateKey(privateKey)
	if err != nil {
		return nil, nil, nil, err
	}
	privKeyPEMBytes := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: privKeyPEM})
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certBytes})

	return certPEM, privKeyPEMBytes, certPEM, nil
}

// TestNewECDSA tests the New function with ECDSA keys
func TestNewECDSA(t *testing.T) {
	pubKey, privKey, rootCA, err := generateTestECDSAKeys()
	if err != nil {
		t.Fatalf("Failed to generate test ECDSA keys: %v", err)
	}

	intercept, err := New(pubKey, privKey, "", rootCA)
	if err != nil {
		t.Fatalf("Failed to create Intercept with ECDSA keys: %v", err)
	}

	if intercept.publicKey == nil || intercept.privateKeyEC == nil || intercept.rootCA == nil {
		t.Fatalf("Intercept object not initialized correctly with ECDSA keys")
	}
}

// TestNewInvalidPublicKey tests the New function with an invalid public key
func TestNewInvalidPublicKey(t *testing.T) {
	_, privKey, rootCA, err := generateTestKeys()
	if err != nil {
		t.Fatalf("Failed to generate test keys: %v", err)
	}

	_, err = New([]byte("invalid public key"), privKey, "", rootCA)
	if err == nil {
		t.Fatalf("Expected error when creating Intercept with invalid public key")
	}
}

// TestNewInvalidPrivateKey tests the New function with an invalid private key
func TestNewInvalidPrivateKey(t *testing.T) {
	pubKey, _, rootCA, err := generateTestKeys()
	if err != nil {
		t.Fatalf("Failed to generate test keys: %v", err)
	}

	_, err = New(pubKey, []byte("invalid private key"), "", rootCA)
	if err == nil {
		t.Fatalf("Expected error when creating Intercept with invalid private key")
	}
}

// generateEncryptedTestKeys generates a new RSA private key and self-signed certificate, and encrypts the private key
func generateEncryptedTestKeys(password string) ([]byte, []byte, error) {
	// Generate a new RSA private key
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, nil, err
	}

	// Encrypt the private key
	privKeyPEMBlock, err := x509.EncryptPEMBlock(rand.Reader, "ENCRYPTED PRIVATE KEY", x509.MarshalPKCS1PrivateKey(privateKey), []byte(password), x509.PEMCipherAES256)
	if err != nil {
		return nil, nil, err
	}
	privKeyPEM := pem.EncodeToMemory(privKeyPEMBlock)

	return privKeyPEM, nil, nil
}

// TestParsePrivateKey tests the parsePrivateKey function
func TestParsePrivateKey(t *testing.T) {
	// Generate test keys
	privKeyPEM, _, err := generateEncryptedTestKeys("password")
	if err != nil {
		t.Fatalf("Failed to generate test keys: %v", err)
	}

	// Test parsing the private key
	privateKey, err := parsePrivateKey(privKeyPEM, "password")
	if err != nil {
		t.Fatalf("Failed to parse private key: %v", err)
	}

	if privateKey == nil {
		t.Fatalf("Expected private key, got nil")
	}
}

// TestParsePrivateKeyInvalid tests the parsePrivateKey function with an invalid private key
func TestParsePrivateKeyInvalid(t *testing.T) {
	_, err := parsePrivateKey([]byte("invalid private key"), "password")
	if err == nil {
		t.Fatalf("Expected error when parsing invalid private key")
	}
}

// TestParsePrivateKeyWrongPassword tests the parsePrivateKey function with a wrong password
func TestParsePrivateKeyWrongPassword(t *testing.T) {
	// Generate test keys
	privKeyPEM, _, err := generateEncryptedTestKeys("password")
	if err != nil {
		t.Fatalf("Failed to generate test keys: %v", err)
	}

	// Test parsing the private key with a wrong password
	_, err = parsePrivateKey(privKeyPEM, "wrongpassword")
	if err == nil {
		t.Fatalf("Expected error when parsing private key with wrong password")
	}
}

// generateEncryptedECDSATestKeys generates a new ECDSA private key and self-signed certificate, and encrypts the private key
func generateEncryptedECDSATestKeys(password string) ([]byte, error) {
	// Generate a new ECDSA private key
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}

	eckey, err := x509.MarshalECPrivateKey(privateKey)
	if err != nil {
		return nil, err
	}

	// Encrypt the private key
	privKeyPEMBlock, err := x509.EncryptPEMBlock(rand.Reader, "EC PRIVATE KEY", eckey, []byte(password), x509.PEMCipherAES256)
	if err != nil {
		return nil, err
	}
	privKeyPEM := pem.EncodeToMemory(privKeyPEMBlock)

	return privKeyPEM, nil
}

// TestParseECDSAPrivateKey tests the parsePrivateKey function with an ECDSA private key
func TestParseECDSAPrivateKey(t *testing.T) {
	// Generate test keys
	privKeyPEM, err := generateEncryptedECDSATestKeys("password")
	if err != nil {
		t.Fatalf("Failed to generate test keys: %v", err)
	}

	// Test parsing the private key
	privateKey, err := parsePrivateKey(privKeyPEM, "password")
	if err != nil {
		t.Fatalf("Failed to parse private key: %v", err)
	}

	if privateKey == nil {
		t.Fatalf("Expected private key, got nil")
	}
}
