package identity

import (
	"github.com/canonical/lxd/shared/api"
)

// CertificateClient represents an identity that authenticates using TLS certificates
// and whose permissions are managed via group membership. It supports both caching
// and fine-grained permissions but is not an admin by default.
type CertificateClient struct {
	typeInfoCommon
}

// AuthenticationMethod indicates that client certificates authenticate using TLS.
func (CertificateClient) AuthenticationMethod() string {
	return api.AuthenticationMethodTLS
}

// IsCacheable indicates that this identity can be cached.
func (CertificateClient) IsCacheable() bool {
	return true
}

// IsFineGrained indicates that this identity uses fine-grained permissions.
func (CertificateClient) IsFineGrained() bool {
	return true
}

// CertificateClientPending represents an identity for which a token has been issued
// but who has not yet authenticated with LXD. It supports fine-grained permissions
// but is not cacheable and not an admin.
type CertificateClientPending struct {
	typeInfoCommon
}

// AuthenticationMethod indicates that pending client certificates authenticate using TLS.
func (CertificateClientPending) AuthenticationMethod() string {
	return api.AuthenticationMethodTLS
}

// IsFineGrained indicates that this identity uses fine-grained permissions.
func (CertificateClientPending) IsFineGrained() bool {
	return true
}

// IsPending indicates that this identity is pending.
func (CertificateClientPending) IsPending() bool {
	return true
}

// CertificateClientRestricted represents an identity that authenticates using TLS certificates
// and is not privileged. It supports caching but does not support fine-grained permissions
// and is not an admin.
type CertificateClientRestricted struct {
	typeInfoCommon
}

// AuthenticationMethod indicates that restricted client certificates authenticate using TLS.
func (CertificateClientRestricted) AuthenticationMethod() string {
	return api.AuthenticationMethodTLS
}

// IsCacheable indicates that this identity can be cached.
func (CertificateClientRestricted) IsCacheable() bool {
	return true
}

// CertificateClientUnrestricted represents an identity that authenticates using TLS certificates
// and is privileged with administrator access. It supports caching, has admin privileges,
// but does not support fine-grained permissions.
type CertificateClientUnrestricted struct {
	typeInfoCommon
}

// AuthenticationMethod indicates that unrestricted client certificates authenticate using TLS.
func (CertificateClientUnrestricted) AuthenticationMethod() string {
	return api.AuthenticationMethodTLS
}

// IsAdmin indicates that this identity type has administrator privileges (unrestricted).
func (CertificateClientUnrestricted) IsAdmin() bool {
	return true
}

// IsCacheable indicates that this identity can be cached.
func (CertificateClientUnrestricted) IsCacheable() bool {
	return true
}
