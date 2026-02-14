package httpsintercept

import (
	"bytes"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"os"
	"slices"
	"testing"
	"time"
)

type testCA struct {
	certPEM []byte
	keyPEM  []byte
	cert    *x509.Certificate
	key     crypto.Signer
}

func newTestCA(t *testing.T, algorithm string, parent *testCA) *testCA {
	t.Helper()

	serial, err := rand.Int(rand.Reader, big.NewInt(1<<62))
	if err != nil {
		t.Fatalf("failed to create serial: %v", err)
	}

	template := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			CommonName: fmt.Sprintf("Test CA %s", algorithm),
		},
		NotBefore:             time.Now().Add(-1 * time.Hour),
		NotAfter:              time.Now().Add(48 * time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IsCA:                  true,
	}

	var key crypto.Signer
	switch algorithm {
	case "rsa":
		keyRSA, err := rsa.GenerateKey(rand.Reader, 2048)
		if err != nil {
			t.Fatalf("failed to generate rsa key: %v", err)
		}
		key = keyRSA
	case "ecdsa":
		keyECDSA, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if err != nil {
			t.Fatalf("failed to generate ecdsa key: %v", err)
		}
		key = keyECDSA
	default:
		t.Fatalf("unsupported algorithm %q", algorithm)
	}

	var parentCert *x509.Certificate
	var parentKey crypto.Signer
	if parent != nil {
		parentCert = parent.cert
		parentKey = parent.key
		template.Issuer = parentCert.Subject
	} else {
		parentCert = template
		parentKey = key
	}

	der, err := x509.CreateCertificate(rand.Reader, template, parentCert, key.Public(), parentKey)
	if err != nil {
		t.Fatalf("failed to create certificate: %v", err)
	}

	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("failed to parse certificate: %v", err)
	}

	var keyDER []byte
	var blockType string
	switch k := key.(type) {
	case *rsa.PrivateKey:
		keyDER = x509.MarshalPKCS1PrivateKey(k)
		blockType = "RSA PRIVATE KEY"
	case *ecdsa.PrivateKey:
		keyDER, err = x509.MarshalECPrivateKey(k)
		if err != nil {
			t.Fatalf("failed to marshal ecdsa key: %v", err)
		}
		blockType = "EC PRIVATE KEY"
	default:
		t.Fatalf("unexpected key type %T", key)
	}

	return &testCA{
		certPEM: pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}),
		keyPEM:  pem.EncodeToMemory(&pem.Block{Type: blockType, Bytes: keyDER}),
		cert:    cert,
		key:     key,
	}
}

func pkcs8PEMFromKey(t *testing.T, key crypto.Signer) []byte {
	t.Helper()
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatalf("failed to marshal pkcs8: %v", err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})
}

func encryptedPEMFromKey(t *testing.T, key crypto.Signer, password, blockType string) []byte {
	t.Helper()
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatalf("failed to marshal pkcs8: %v", err)
	}
	block, err := x509.EncryptPEMBlock(rand.Reader, blockType, der, []byte(password), x509.PEMCipherAES256) //nolint:staticcheck // test fixture for legacy encrypted PEM support
	if err != nil {
		t.Fatalf("failed to encrypt pem block: %v", err)
	}
	return pem.EncodeToMemory(block)
}

func TestParsePublicKey(t *testing.T) {
	ca := newTestCA(t, "rsa", nil)

	got, err := parsePublicKey(ca.certPEM)
	if err != nil {
		t.Fatalf("parsePublicKey returned error: %v", err)
	}
	if !bytes.Equal(got.Raw, ca.cert.Raw) {
		t.Fatalf("parsed certificate mismatch")
	}
}

func TestParsePublicKeyInvalid(t *testing.T) {
	_, err := parsePublicKey([]byte("not a certificate"))
	if !errors.Is(err, ErrInvalidPublicKey) {
		t.Fatalf("expected ErrInvalidPublicKey, got %v", err)
	}
}

func TestParsePrivateKeyRSA(t *testing.T) {
	ca := newTestCA(t, "rsa", nil)

	key, err := parsePrivateKey(ca.keyPEM, "")
	if err != nil {
		t.Fatalf("parsePrivateKey returned error: %v", err)
	}
	if _, ok := key.(*rsa.PrivateKey); !ok {
		t.Fatalf("expected RSA private key, got %T", key)
	}
}

func TestParsePrivateKeyECDSA(t *testing.T) {
	ca := newTestCA(t, "ecdsa", nil)
	pkcs8 := pkcs8PEMFromKey(t, ca.key)

	key, err := parsePrivateKey(pkcs8, "")
	if err != nil {
		t.Fatalf("parsePrivateKey returned error: %v", err)
	}
	if _, ok := key.(*ecdsa.PrivateKey); !ok {
		t.Fatalf("expected ECDSA private key, got %T", key)
	}
}

func TestParsePrivateKeyEncrypted(t *testing.T) {
	ca := newTestCA(t, "ecdsa", nil)
	pemBytes := encryptedPEMFromKey(t, ca.key, "secret", "ENCRYPTED PRIVATE KEY")

	key, err := parsePrivateKey(pemBytes, "secret")
	if err != nil {
		t.Fatalf("parsePrivateKey returned error: %v", err)
	}
	if _, ok := key.(*ecdsa.PrivateKey); !ok {
		t.Fatalf("expected ECDSA private key, got %T", key)
	}
}

func TestParsePrivateKeyUnsupportedEncryptedType(t *testing.T) {
	ca := newTestCA(t, "rsa", nil)
	pemBytes := encryptedPEMFromKey(t, ca.key, "secret", "RSA PRIVATE KEY")

	_, err := parsePrivateKey(pemBytes, "secret")
	if !errors.Is(err, ErrInvalidPrivateKey) {
		t.Fatalf("expected ErrInvalidPrivateKey, got %v", err)
	}
}

func TestParsePrivateKeyInvalidData(t *testing.T) {
	_, err := parsePrivateKey([]byte("bad data"), "")
	if err == nil {
		t.Fatalf("expected error for invalid private key data")
	}
}

func TestParsePrivateKeyWrongPassword(t *testing.T) {
	ca := newTestCA(t, "ecdsa", nil)
	pemBytes := encryptedPEMFromKey(t, ca.key, "secret", "ENCRYPTED PRIVATE KEY")

	_, err := parsePrivateKey(pemBytes, "wrong")
	if err == nil {
		t.Fatalf("expected error for wrong password")
	}
}

func TestParseRootCANil(t *testing.T) {
	cert, err := parseRootCA(nil)
	if !errors.Is(err, ErrRootCANotProvided) {
		t.Fatalf("expected ErrRootCANotProvided, got %v", err)
	}
	if cert != nil {
		t.Fatalf("expected nil certificate for nil input")
	}
}

func TestParseRootCAInvalid(t *testing.T) {
	_, err := parseRootCA([]byte("invalid"))
	if !errors.Is(err, ErrInvalidPublicKey) {
		t.Fatalf("expected ErrInvalidPublicKey, got %v", err)
	}
}

func TestParseRootCASuccess(t *testing.T) {
	root := newTestCA(t, "rsa", nil)

	cert, err := parseRootCA(root.certPEM)
	if err != nil {
		t.Fatalf("parseRootCA returned error: %v", err)
	}
	if !bytes.Equal(cert.Raw, root.cert.Raw) {
		t.Fatalf("parsed root certificate mismatch")
	}
}

func TestNewWithRSA(t *testing.T) {
	ca := newTestCA(t, "rsa", nil)

	intercept, err := New(ca.certPEM, ca.keyPEM, "", ca.certPEM)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	if intercept.privateKey == nil {
		t.Fatalf("expected RSA private key to be set")
	}
	if intercept.privateKeyEC != nil {
		t.Fatalf("expected privateKeyEC to be nil")
	}
	if intercept.rootCA == nil {
		t.Fatalf("expected rootCA to be set")
	}
}

func TestNewWithECDSA(t *testing.T) {
	root := newTestCA(t, "rsa", nil)
	intermediate := newTestCA(t, "ecdsa", root)

	intercept, err := New(intermediate.certPEM, intermediate.keyPEM, "", root.certPEM)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	if intercept.privateKeyEC == nil {
		t.Fatalf("expected ECDSA private key to be set")
	}
	if intercept.privateKey != nil {
		t.Fatalf("expected privateKey to be nil")
	}
	if intercept.rootCA == nil || !bytes.Equal(intercept.rootCA.Raw, root.cert.Raw) {
		t.Fatalf("expected rootCA to match provided root certificate")
	}
}

func TestNewInvalidPublicKey(t *testing.T) {
	ca := newTestCA(t, "rsa", nil)
	_, err := New([]byte("bad"), ca.keyPEM, "", ca.certPEM)
	if err == nil {
		t.Fatalf("expected error for invalid public key input")
	}
}

func TestNewInvalidPrivateKey(t *testing.T) {
	ca := newTestCA(t, "rsa", nil)
	_, err := New(ca.certPEM, []byte("bad"), "", ca.certPEM)
	if err == nil {
		t.Fatalf("expected error for invalid private key input")
	}
}

func TestCreateInterceptWithRSA(t *testing.T) {
	ca := newTestCA(t, "rsa", nil)
	intercept, err := createIntercept(ca.cert, ca.key, nil)
	if err != nil {
		t.Fatalf("createIntercept returned error: %v", err)
	}
	if intercept.privateKey == nil || intercept.privateKeyEC != nil {
		t.Fatalf("unexpected private key assignment")
	}
}

func TestCreateInterceptWithECDSA(t *testing.T) {
	ca := newTestCA(t, "ecdsa", nil)
	intercept, err := createIntercept(ca.cert, ca.key, nil)
	if err != nil {
		t.Fatalf("createIntercept returned error: %v", err)
	}
	if intercept.privateKeyEC == nil || intercept.privateKey != nil {
		t.Fatalf("unexpected private key assignment")
	}
}

func TestCreateInterceptInvalidKeyType(t *testing.T) {
	ca := newTestCA(t, "rsa", nil)
	_, err := createIntercept(ca.cert, struct{}{}, nil)
	if err == nil {
		t.Fatalf("expected error for unsupported key type")
	}
}

func TestSetterMethods(t *testing.T) {
	ca := newTestCA(t, "rsa", nil)
	intercept, err := New(ca.certPEM, ca.keyPEM, "", ca.certPEM)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	intercept.SetAIAAddress("http://aia.local")
	intercept.SetCRLAddress("http://crl.local")
	intercept.SetDomain("default.local")

	if intercept.aiaAddress != "http://aia.local" {
		t.Fatalf("unexpected AIA address %q", intercept.aiaAddress)
	}
	if intercept.crlAddress != "http://crl.local" {
		t.Fatalf("unexpected CRL address %q", intercept.crlAddress)
	}
	if intercept.domain != "default.local" {
		t.Fatalf("unexpected domain %q", intercept.domain)
	}
}

func TestGenerateProxyCertificateAddsSANAndChain(t *testing.T) {
	root := newTestCA(t, "rsa", nil)
	intermediate := newTestCA(t, "ecdsa", root)
	intercept, err := createIntercept(intermediate.cert, intermediate.key, root.cert)
	if err != nil {
		t.Fatalf("createIntercept returned error: %v", err)
	}
	intercept.SetDomain("default.local")
	intercept.SetAIAAddress("http://aia.example/aia")
	intercept.SetCRLAddress("http://crl.example/crl")

	cert, err := intercept.generateProxyCertificate("www.example.com")
	if err != nil {
		t.Fatalf("generateProxyCertificate returned error: %v", err)
	}
	if cert == nil || cert.Leaf == nil {
		t.Fatalf("expected non-nil certificate with parsed leaf")
	}
	if !slices.Contains(cert.Leaf.DNSNames, "www.example.com") {
		t.Fatalf("expected requested hostname in DNS SANs")
	}
	if !slices.Contains(cert.Leaf.DNSNames, "default.local") {
		t.Fatalf("expected default domain in DNS SANs")
	}
	if len(cert.Certificate) != 3 {
		t.Fatalf("expected certificate chain length 3, got %d", len(cert.Certificate))
	}
	if len(cert.Leaf.IssuingCertificateURL) != 1 || cert.Leaf.IssuingCertificateURL[0] != "http://aia.example/aia" {
		t.Fatalf("unexpected issuing certificate URL")
	}
	if len(cert.Leaf.CRLDistributionPoints) != 1 || cert.Leaf.CRLDistributionPoints[0] != "http://crl.example/crl" {
		t.Fatalf("unexpected CRL distribution points")
	}
	if cert.PrivateKey == nil {
		t.Fatalf("expected generated certificate to include private key")
	}
}

func TestGenerateProxyCertificateAddsDomainIP(t *testing.T) {
	root := newTestCA(t, "rsa", nil)
	intermediate := newTestCA(t, "rsa", root)
	intercept, err := createIntercept(intermediate.cert, intermediate.key, root.cert)
	if err != nil {
		t.Fatalf("createIntercept returned error: %v", err)
	}
	intercept.SetDomain("127.0.0.1:8443")

	cert, err := intercept.generateProxyCertificate("192.0.2.1")
	if err != nil {
		t.Fatalf("generateProxyCertificate returned error: %v", err)
	}
	if len(cert.Leaf.IPAddresses) != 2 {
		t.Fatalf("expected two IP SANs, got %d", len(cert.Leaf.IPAddresses))
	}
}

func TestGenerateProxyCertificateNormalizesDomainHostPort(t *testing.T) {
	root := newTestCA(t, "rsa", nil)
	intermediate := newTestCA(t, "rsa", root)
	intercept, err := createIntercept(intermediate.cert, intermediate.key, root.cert)
	if err != nil {
		t.Fatalf("createIntercept returned error: %v", err)
	}
	intercept.SetDomain("www.example.com:8443")

	cert, err := intercept.generateProxyCertificate("www.example.com")
	if err != nil {
		t.Fatalf("generateProxyCertificate returned error: %v", err)
	}

	seen := 0
	for _, dnsName := range cert.Leaf.DNSNames {
		if dnsName == "www.example.com" {
			seen++
		}
		if dnsName == "www.example.com:8443" {
			t.Fatalf("expected host:port to be normalized before SAN insertion")
		}
	}
	if seen != 1 {
		t.Fatalf("expected deduplicated DNS SAN for normalized host, got %d", seen)
	}
}

func TestGenerateProxyCertificateNormalizesBracketedIPv6WithPort(t *testing.T) {
	root := newTestCA(t, "rsa", nil)
	intermediate := newTestCA(t, "ecdsa", root)
	intercept, err := createIntercept(intermediate.cert, intermediate.key, root.cert)
	if err != nil {
		t.Fatalf("createIntercept returned error: %v", err)
	}
	intercept.SetDomain("[2001:db8::1]:8443")

	cert, err := intercept.generateProxyCertificate("2001:db8::1")
	if err != nil {
		t.Fatalf("generateProxyCertificate returned error: %v", err)
	}
	if len(cert.Leaf.IPAddresses) != 1 {
		t.Fatalf("expected one deduplicated IPv6 SAN, got %d", len(cert.Leaf.IPAddresses))
	}
	if got := cert.Leaf.IPAddresses[0].String(); got != "2001:db8::1" {
		t.Fatalf("unexpected IPv6 SAN %q", got)
	}
}

func TestCreateCertificateStoresAndResetsOperation(t *testing.T) {
	root := newTestCA(t, "rsa", nil)
	intermediate := newTestCA(t, "ecdsa", root)
	intercept, err := createIntercept(intermediate.cert, intermediate.key, root.cert)
	if err != nil {
		t.Fatalf("createIntercept returned error: %v", err)
	}

	if err := intercept.CreateCertificate("cache.example"); err != nil {
		t.Fatalf("CreateCertificate returned error: %v", err)
	}
	intercept.certStorage.mutex.RLock()
	cert, ok := intercept.certStorage.Certificates["cache.example"]
	intercept.certStorage.mutex.RUnlock()
	if !ok {
		t.Fatalf("certificate not stored for domain")
	}
	if cert.OperationInProgress {
		t.Fatalf("operation flag not cleared")
	}
	if cert.Certificate == nil || cert.Certificate.Leaf == nil {
		t.Fatalf("stored certificate invalid")
	}
}

func TestGetCertificateReturnsCachedInstance(t *testing.T) {
	root := newTestCA(t, "rsa", nil)
	intermediate := newTestCA(t, "rsa", root)
	intercept, err := createIntercept(intermediate.cert, intermediate.key, root.cert)
	if err != nil {
		t.Fatalf("createIntercept returned error: %v", err)
	}

	first := intercept.GetCertificate("example.com")
	if first == nil {
		t.Fatalf("expected certificate on first retrieval")
	}
	second := intercept.GetCertificate("example.com")
	if second == nil || first != second {
		t.Fatalf("expected cached certificate instance")
	}
}

func TestReturnCertFallsBackToConfiguredDomain(t *testing.T) {
	root := newTestCA(t, "rsa", nil)
	intermediate := newTestCA(t, "ecdsa", root)
	intercept, err := createIntercept(intermediate.cert, intermediate.key, root.cert)
	if err != nil {
		t.Fatalf("createIntercept returned error: %v", err)
	}
	intercept.SetDomain("fallback.example")

	cert, err := intercept.ReturnCert(&tls.ClientHelloInfo{ServerName: ""})
	if err != nil {
		t.Fatalf("ReturnCert returned error: %v", err)
	}
	if cert == nil {
		t.Fatalf("expected fallback certificate")
	}
}

func TestCertificateStorageOperationTracking(t *testing.T) {
	storage := certificateStorage{
		Certificates: make(map[string]IssuedCertificate),
	}
	storage.Certificates["example.com"] = IssuedCertificate{}

	if storage.IsInOperation("example.com") {
		t.Fatalf("unexpected operation flag before start")
	}
	storage.StartOperation("example.com")
	if !storage.IsInOperation("example.com") {
		t.Fatalf("expected operation flag after start")
	}
	storage.EndOperation("example.com")
	if storage.IsInOperation("example.com") {
		t.Fatalf("expected operation flag to be cleared")
	}
}

func TestGCRemovesExpiringCertificates(t *testing.T) {
	root := newTestCA(t, "rsa", nil)
	intermediate := newTestCA(t, "rsa", root)
	intercept, err := createIntercept(intermediate.cert, intermediate.key, root.cert)
	if err != nil {
		t.Fatalf("createIntercept returned error: %v", err)
	}

	intercept.certStorage.Certificates["soon.example"] = IssuedCertificate{
		Certificate: &tls.Certificate{},
		Expires:     time.Now().Add(30 * time.Second),
	}
	intercept.certStorage.Certificates["later.example"] = IssuedCertificate{
		Certificate: &tls.Certificate{},
		Expires:     time.Now().Add(10 * time.Minute),
	}

	intercept.GC()

	intercept.certStorage.mutex.RLock()
	_, soonExists := intercept.certStorage.Certificates["soon.example"]
	_, laterExists := intercept.certStorage.Certificates["later.example"]
	intercept.certStorage.mutex.RUnlock()

	if soonExists {
		t.Fatalf("expected soon expiring certificate to be removed")
	}
	if !laterExists {
		t.Fatalf("expected later certificate to remain")
	}
}

func TestGenerateCRLWritesParsableFile(t *testing.T) {
	root := newTestCA(t, "rsa", nil)
	intermediate := newTestCA(t, "ecdsa", root)
	intercept, err := createIntercept(intermediate.cert, intermediate.key, root.cert)
	if err != nil {
		t.Fatalf("createIntercept returned error: %v", err)
	}

	dir := t.TempDir()
	path := dir + "/crl.der"
	if err := intercept.GenerateCRL("http://crl.example", path); err != nil {
		t.Fatalf("GenerateCRL returned error: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read generated CRL: %v", err)
	}
	if len(data) == 0 {
		t.Fatalf("expected CRL data to be written")
	}
	if _, err := x509.ParseRevocationList(data); err != nil {
		t.Fatalf("generated CRL is not parsable: %v", err)
	}
}

func TestGenKeyPairProducesECDSAKey(t *testing.T) {
	key, err := genKeyPair()
	if err != nil {
		t.Fatalf("genKeyPair returned error: %v", err)
	}
	if key == nil || key.Curve != elliptic.P256() {
		t.Fatalf("expected P256 key from genKeyPair")
	}
}
