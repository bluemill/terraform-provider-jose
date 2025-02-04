package joseprovider

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"errors"
	"fmt"

	"github.com/go-jose/go-jose"
)

// Had to copy-paste this due to the wrong import paths defined due to the move from square => go-jose

// See https://github.com/go-jose/go-jose/blob/master/jose-util/generator/generate.go
// and https://github.com/go-jose/go-jose/blob/master/jose-util/generate.go

// NewSigningKey generates a keypair for corresponding SignatureAlgorithm.
func NewSigningKey(alg jose.SignatureAlgorithm, bits int) (crypto.PublicKey, crypto.PrivateKey, error) {
	switch alg {
	case jose.ES256, jose.ES384, jose.ES512, jose.EdDSA:
		keylen := map[jose.SignatureAlgorithm]int{
			jose.ES256: 256,
			jose.ES384: 384,
			jose.ES512: 521, // sic!
			jose.EdDSA: 256,
		}
		if bits != 0 && bits != keylen[alg] {
			return nil, nil, errors.New("invalid elliptic curve key size, this algorithm does not support arbitrary size")
		}
	case jose.RS256, jose.RS384, jose.RS512, jose.PS256, jose.PS384, jose.PS512:
		if bits == 0 {
			bits = 2048
		}
		if bits < 2048 {
			return nil, nil, errors.New("invalid key size for RSA key, 2048 or more is required")
		}
	}
	switch alg {
	case jose.ES256:
		key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if err != nil {
			return nil, nil, err
		}
		return key.Public(), key, err
	case jose.ES384:
		key, err := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
		if err != nil {
			return nil, nil, err
		}
		return key.Public(), key, err
	case jose.ES512:
		key, err := ecdsa.GenerateKey(elliptic.P521(), rand.Reader)
		if err != nil {
			return nil, nil, err
		}
		return key.Public(), key, err
	case jose.EdDSA:
		pub, key, err := ed25519.GenerateKey(rand.Reader)
		return pub, key, err
	case jose.RS256, jose.RS384, jose.RS512, jose.PS256, jose.PS384, jose.PS512:
		key, err := rsa.GenerateKey(rand.Reader, bits)
		if err != nil {
			return nil, nil, err
		}
		return key.Public(), key, err
	default:
		return nil, nil, fmt.Errorf("unknown algorithm %s for signing key", alg)
	}
}

// NewEncryptionKey generates a keypair for corresponding KeyAlgorithm.
func NewEncryptionKey(alg jose.KeyAlgorithm, bits int) (crypto.PublicKey, crypto.PrivateKey, error) {
	switch alg {
	case jose.RSA1_5, jose.RSA_OAEP, jose.RSA_OAEP_256:
		if bits == 0 {
			bits = 2048
		}
		if bits < 2048 {
			return nil, nil, errors.New("invalid key size for RSA key, 2048 or more is required")
		}
		key, err := rsa.GenerateKey(rand.Reader, bits)
		if err != nil {
			return nil, nil, err
		}
		return key.Public(), key, err
	case jose.ECDH_ES, jose.ECDH_ES_A128KW, jose.ECDH_ES_A192KW, jose.ECDH_ES_A256KW:
		var crv elliptic.Curve
		switch bits {
		case 0, 256:
			crv = elliptic.P256()
		case 384:
			crv = elliptic.P384()
		case 521:
			crv = elliptic.P521()
		default:
			return nil, nil, errors.New("invalid elliptic curve key size, use one of 256, 384, or 521")
		}
		key, err := ecdsa.GenerateKey(crv, rand.Reader)
		if err != nil {
			return nil, nil, err
		}
		return key.Public(), key, err
	default:
		return nil, nil, fmt.Errorf("unknown algorithm %s for encryption key", alg)
	}
}

func generateKey(use string, alg string, size int) (pubJSON string, privJSON string, kid string, err error) {
	var privKey crypto.PrivateKey
	var pubKey crypto.PublicKey

	switch use {
	case "sig":
		pubKey, privKey, err = NewSigningKey(jose.SignatureAlgorithm(alg), size)
	case "enc":
		pubKey, privKey, err = NewEncryptionKey(jose.KeyAlgorithm(alg), size)
	default:
		// According to RFC 7517 section-8.2.  This is unlikely to change in the
		// near future. If it were, new values could be found in the registry under
		// "JSON Web Key Use": https://www.iana.org/assignments/jose/jose.xhtml
		return "", "", "", fmt.Errorf("invalid key use.  Must be 'sig' or 'enc'")
	}
	if err != nil {
		return "", "", "", fmt.Errorf("error when generating keyset: %v", err)
	}

	priv := jose.JSONWebKey{Key: privKey, KeyID: kid, Algorithm: alg, Use: use}
	// Generate a canonical kid based on RFC 7638
	thumb, err := priv.Thumbprint(crypto.SHA256)
	if err != nil {
		return "", "", "", fmt.Errorf("unable to compute thumbprint: %v", err)
	}

	kid = base64.URLEncoding.EncodeToString(thumb)
	priv.KeyID = kid

	// I'm not sure why we couldn't use `pub := priv.Public()` here as the private
	// key should contain the public key.  In case for some reason it doesn't,
	// this builds a public JWK from scratch.
	pub := jose.JSONWebKey{Key: pubKey, KeyID: kid, Algorithm: alg, Use: use}

	if priv.IsPublic() || !pub.IsPublic() || !priv.Valid() || !pub.Valid() {
		return "", "", "", errors.New("invalid keys were generated")
	}

	privJSONbs, err := priv.MarshalJSON()
	if err != nil {
		return "", "", "", errors.New("failed to marshal private key to JSON")
	}

	pubJSONbs, err := pub.MarshalJSON()
	if err != nil {
		return "", "", "", errors.New("failed to marshal public key to JSON")
	}

	return string(pubJSONbs), string(privJSONbs), kid, nil
}
