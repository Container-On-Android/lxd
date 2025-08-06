package identity

import (
	"github.com/canonical/lxd/shared/api"
)

// CertificateServer represents cluster member authentication using TLS certificates.
// It has administrator privileges and supports caching but does not support fine-grained permissions.
type CertificateServer struct {
	typeInfoCommon
}

// AuthenticationMethod indicates that server certificates authenticate using TLS.
func (CertificateServer) AuthenticationMethod() string {
	return api.AuthenticationMethodTLS
}

// IsAdmin indicates that this identity type has administrator privileges (unrestricted).
func (CertificateServer) IsAdmin() bool {
	return true
}

// IsCacheable indicates that this identity can be cached.
func (CertificateServer) IsCacheable() bool {
	return true
}
