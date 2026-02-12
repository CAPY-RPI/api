package service_test

import (
	"testing"

	"github.com/capyrpi/api/internal/auth/service"
	"github.com/stretchr/testify/assert"
)

func TestAuthService(t *testing.T) {
	// Just a dummy test to ensure the package imports are correct
	// and the struct can be instantiated (with nil dependencies for now)
	svc := service.NewAuthService(nil, nil, nil, nil)
	assert.NotNil(t, svc)
}
