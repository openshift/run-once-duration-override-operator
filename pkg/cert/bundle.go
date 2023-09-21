package cert

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
)

// Bundle encapsulates
// - PEM encoded serving private key and certificate
// - certificate of the self-signed CA that signed the serving cert.
type Bundle struct {
	Serving
	ServingCertCA []byte
}

type Serving struct {
	ServiceKey  []byte
	ServiceCert []byte
}

func (b *Bundle) Validate() error {
	if len(b.ServingCertCA) == 0 {
		return errors.New("serving service cert CA must be specified")
	}

	if len(b.Serving.ServiceCert) == 0 {
		return errors.New("serving service cert must be specified")
	}

	if len(b.Serving.ServiceKey) == 0 {
		return errors.New("serving service private key must be specified")
	}

	return nil
}

// Hash generates a sha256 hash of the given Bundle object
// The hash is generated from the hash of the serving key, serving cert, serving CA cert.
func (b *Bundle) Hash() string {
	writer := sha256.New()

	_, err := writer.Write(b.ServiceKey)
	if err != nil {
		return ""
	}
	h1 := writer.Sum(nil)

	writer.Reset()
	_, err = writer.Write(b.ServiceCert)
	if err != nil {
		return ""
	}
	h2 := writer.Sum(h1)

	writer.Reset()
	_, err = writer.Write(b.ServingCertCA)
	if err != nil {
		return ""
	}
	h := writer.Sum(h2)

	writer.Reset()
	_, err = writer.Write(h)
	if err != nil {
		return ""
	}

	return hex.EncodeToString(writer.Sum(nil))
}
