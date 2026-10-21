package keyring

import (
	"errors"

	gokeyring "github.com/zalando/go-keyring"

	"github.com/dscafati/secretref/internal/provider"
)

// Backend is the OS keyring as the Provider uses it. Get and Delete return an
// error wrapping provider.ErrNotFound when no entry exists. Calls take no
// context: the Provider bounds them itself.
type Backend interface {
	Get(service, user string) (string, error)
	Set(service, user, password string) error
	Delete(service, user string) error
}

var _ Backend = osBackend{}

// osBackend is the Backend of the host OS, through zalando/go-keyring.
type osBackend struct{}

func (osBackend) Get(service, user string) (string, error) {
	v, err := gokeyring.Get(service, user)
	return v, notFound(err)
}

func (osBackend) Set(service, user, password string) error {
	return gokeyring.Set(service, user, password)
}

func (osBackend) Delete(service, user string) error {
	return notFound(gokeyring.Delete(service, user))
}

// notFound translates go-keyring's missing-entry error into the Backend
// contract and leaves every other error as it is.
func notFound(err error) error {
	if errors.Is(err, gokeyring.ErrNotFound) {
		return provider.ErrNotFound
	}
	return err
}
