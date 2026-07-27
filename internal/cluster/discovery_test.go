package cluster

import (
	"net"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAdvertiseAddress_JoinsPort(t *testing.T) {
	addr, err := AdvertiseAddress(nil, 7946)
	if err != nil {
		// No usable interface in this environment (e.g. CI without eth0/en0).
		t.Skipf("advertise address unavailable: %v", err)
	}
	_, port, err := net.SplitHostPort(addr)
	require.NoError(t, err)
	require.Equal(t, strconv.Itoa(7946), port)
}
