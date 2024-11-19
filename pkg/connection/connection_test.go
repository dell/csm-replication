package connection

import (
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
)

func TestConnect(t *testing.T) {
	// Test case: Successful connection
	address := "unix:///path/to/socket"
	log := logr.Discard()

	conn, err := Connect(address, log)
	assert.NoError(t, err)
	assert.NotNil(t, conn)

	// Test case: Missing prefix
	address = "/path/to/socket"
	conn, err = Connect(address, log)
	assert.NoError(t, err)
	assert.NotNil(t, conn)
}
