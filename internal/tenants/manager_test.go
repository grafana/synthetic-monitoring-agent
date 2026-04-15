package tenants

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/grafana/synthetic-monitoring-agent/internal/cache"
	sm "github.com/grafana/synthetic-monitoring-agent/pkg/pb/synthetic_monitoring"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
)

type testTenantsClient struct {
	tenants      map[int64]sm.Tenant
	requestCount map[int64]int
	err          error
}

var errTenantNotFound = errors.New("tenant not found")

func (c *testTenantsClient) GetTenant(ctx context.Context, in *sm.TenantInfo, opts ...grpc.CallOption) (*sm.Tenant, error) {
	c.requestCount[in.Id]++

	if c.err != nil {
		return nil, c.err
	}

	tenant, found := c.tenants[in.Id]
	if !found {
		return nil, errTenantNotFound
	}

	return &tenant, nil
}

func makeTenant(idx int) sm.Tenant {
	return sm.Tenant{
		Id:    int64(idx),
		OrgId: int64(idx * 1000),
		MetricsRemote: &sm.RemoteInfo{
			Name:     fmt.Sprintf("test-%d", idx),
			Url:      fmt.Sprintf("http://127.0.0.1/%d", idx),
			Username: fmt.Sprintf("user-%d", idx),
			Password: fmt.Sprintf("pw-%d", idx),
		},
	}
}

func TestTenantManagerGetTenant(t *testing.T) {
	tc := testTenantsClient{
		tenants: map[int64]sm.Tenant{
			1: makeTenant(1),
		},
		requestCount: make(map[int64]int),
	}

	deadline, hasTimeout := t.Deadline()
	if !hasTimeout {
		deadline = time.Now().Add(5 * time.Second)
	}

	ctx, cancel := context.WithDeadline(context.Background(), deadline)
	defer cancel()

	tenantCh := make(chan sm.Tenant)
	cacheExpirationTime := 200 * time.Millisecond
	maxCacheExpirationTime := cacheExpirationTime + time.Duration(defaultCacheJitter*float64(cacheExpirationTime))
	logger := zerolog.New(zerolog.NewTestWriter(t))

	// Create a local cache for testing
	// Use DefaultTTL = 0 to disable automatic expiration, allowing us to test
	// the stale data fallback behavior (ValidUntil is checked manually)
	localCache, err := cache.NewLocal(cache.LocalConfig{
		MaxCapacity:     100,
		InitialCapacity: 10,
		DefaultTTL:      0, // Disable automatic expiration
		Logger:          logger,
	})
	require.NoError(t, err)
	defer localCache.Close()

	tm := NewManager(ctx, &tc, tenantCh, cacheExpirationTime, localCache, logger)

	t1 := tc.tenants[1]

	// requesting an existent tenant should return it
	tenant, err := tm.GetTenant(ctx, &sm.TenantInfo{Id: t1.Id})
	require.NoError(t, err)
	require.NotNil(t, tenant)
	require.Equal(t, 1, tc.requestCount[t1.Id])
	require.Equal(t, t1, *tenant)

	// requesting the same tenant within a short period of time
	// should not cause a new request to the server, so the count
	// should remain at 1.
	tenant, err = tm.GetTenant(ctx, &sm.TenantInfo{Id: t1.Id})
	require.NoError(t, err)
	require.NotNil(t, tenant)
	require.Equal(t, 1, tc.requestCount[t1.Id])
	require.Equal(t, t1, *tenant)

	// requesting the same tenant after a longer time should evict
	// the existing tenant and make a new request; make sure we are
	// not getting a cached copy.
	time.Sleep(maxCacheExpirationTime)

	t1.MetricsRemote.Password += "-new"

	tenant, err = tm.GetTenant(ctx, &sm.TenantInfo{Id: t1.Id})
	require.NoError(t, err)
	require.NotNil(t, tenant)
	require.Equal(t, 2, tc.requestCount[t1.Id])
	require.Equal(t, t1, *tenant)

	// create a new tenant but don't insert it in the cache
	t2 := makeTenant(2)

	// requesting a non-existent tenant should return an error
	tenant, err = tm.GetTenant(ctx, &sm.TenantInfo{Id: t2.Id})
	require.Error(t, err)
	require.Equal(t, errTenantNotFound, err)
	require.Nil(t, tenant)
	require.Equal(t, 1, tc.requestCount[t2.Id])

	// negative responses should not be cached
	tenant, err = tm.GetTenant(ctx, &sm.TenantInfo{Id: t2.Id})
	require.Error(t, err)
	require.Equal(t, errTenantNotFound, err)
	require.Nil(t, tenant)
	require.Equal(t, 2, tc.requestCount[t2.Id])

	// after adding the tenant, a new request for that tenant should
	// return the correct information
	tc.tenants[2] = t2
	tenant, err = tm.GetTenant(ctx, &sm.TenantInfo{Id: t2.Id})
	require.NoError(t, err)
	require.NotNil(t, tenant)
	require.Equal(t, 3, tc.requestCount[t2.Id])
	require.Equal(t, t2, *tenant)

	// create a new tenant and send it over the channel to the
	// tenant manager
	t3 := makeTenant(3)

	tenantCh <- t3
	// Give the manager goroutine some time to process the channel message
	// before we start checking. This reduces unnecessary API calls.
	time.Sleep(10 * time.Millisecond)

	// Now check if the tenant was added to the cache via the channel
	// by verifying that GetTenant succeeds without making an API request
	tenant, err = tm.GetTenant(ctx, &sm.TenantInfo{Id: t3.Id})
	require.NoError(t, err)
	require.NotNil(t, tenant)
	require.Equal(t, 0, tc.requestCount[t3.Id], "should not have made API request for tenant received via channel")
	require.Equal(t, t3, *tenant)

	// wait for tenants to expire
	time.Sleep(maxCacheExpirationTime)
	// force tenants client to return an error
	tc.err = errors.New("network error")

	// if a tenant is present in the cache, this one should be returned, even
	// if it is expired, in case new data can not be retrieved from the API
	tenant, err = tm.GetTenant(ctx, &sm.TenantInfo{Id: t3.Id})
	require.NoError(t, err)
	require.NotNil(t, tenant)
	require.Equal(t, 1, tc.requestCount[t3.Id])
	require.Equal(t, t3, *tenant)
}

func TestCalculateValidUntil(t *testing.T) {
	var (
		now = time.Now()
		// compute the relative error if there's a 1 second difference
		// between the time we expect (which is around now+timeout) and
		// a time that is 1 second later. In other words, tolare a
		// relative difference of about 1 second in the results we get.
		defEpsilon = float64(now.Add(1*time.Second).UnixNano()-now.UnixNano()) / float64(now.UnixNano())
		// For cases on which the secret store expiraiton is not taken into
		// account, a jitter is applied to the timeout.
		jitterEpsilon = defaultCacheJitter
	)

	// Some tests below assume this is true.
	require.Greater(t, sm.MaxScriptedTimeout, 2*time.Minute)
	require.Less(t, sm.MaxScriptedTimeout, 5*time.Minute)

	testCases := map[string]struct {
		timeout time.Duration
		tenant  *sm.Tenant
		want    time.Duration
		epsilon float64
	}{
		"10 minute timeout, no secret store": {
			timeout: 10 * time.Minute,
			tenant:  &sm.Tenant{},
			want:    10 * time.Minute,
			epsilon: jitterEpsilon,
		},
		"1 hour timeout, no secret store": {
			timeout: 1 * time.Hour,
			tenant:  &sm.Tenant{},
			want:    1 * time.Hour,
			epsilon: jitterEpsilon,
		},
		"7.5 minute timeout, secret store expires in 2 minutes (less than MaxScriptedTimeout)": {
			timeout: 7*time.Minute + 30*time.Second,
			tenant: &sm.Tenant{
				SecretStore: &sm.SecretStore{
					Url:    "https://secrets.example.com",
					Token:  "token",
					Expiry: float64(now.Add(2*time.Minute).UnixNano()) / 1e9,
				},
			},
			want:    2 * time.Minute,
			epsilon: defEpsilon,
		},
		"7.5 minute timeout, secret store expires in 5 minutes (more than MaxScriptedTimeout)": {
			timeout: 7*time.Minute + 30*time.Second,
			tenant: &sm.Tenant{
				SecretStore: &sm.SecretStore{
					Url:    "https://secrets.example.com",
					Token:  "token",
					Expiry: float64(now.Add(5*time.Minute).UnixNano()) / 1e9,
				},
			},
			want:    5*time.Minute - sm.MaxScriptedTimeout,
			epsilon: defEpsilon,
		},
		"7.5 minute timeout, secret store expires in 1 hour": {
			timeout: 7*time.Minute + 30*time.Second,
			tenant: &sm.Tenant{
				SecretStore: &sm.SecretStore{
					Url:    "https://secrets.example.com",
					Token:  "token",
					Expiry: float64(now.Add(1*time.Hour).UnixNano()) / 1e9,
				},
			},
			want:    7*time.Minute + 30*time.Second,
			epsilon: defEpsilon,
		},
		// This should not make a difference wrt to the previous tests. Make sure that's the case.
		"10 minute timeout, secret store expires in 2 minutes": {
			timeout: 10 * time.Minute,
			tenant: &sm.Tenant{
				SecretStore: &sm.SecretStore{
					Url:    "https://secrets.example.com",
					Token:  "token",
					Expiry: float64(now.Add(2*time.Minute).UnixNano()) / 1e9,
				},
			},
			want:    2 * time.Minute,
			epsilon: defEpsilon,
		},
		"10 minute timeout, secret store expires in 5 minutes": {
			timeout: 10 * time.Minute,
			tenant: &sm.Tenant{
				SecretStore: &sm.SecretStore{
					Url:    "https://secrets.example.com",
					Token:  "token",
					Expiry: float64(now.Add(5*time.Minute).UnixNano()) / 1e9,
				},
			},
			want:    5*time.Minute - sm.MaxScriptedTimeout,
			epsilon: defEpsilon,
		},
		"10 minute timeout, secret store expires in 10 minutes": {
			timeout: 10 * time.Minute,
			tenant: &sm.Tenant{
				SecretStore: &sm.SecretStore{
					Url:    "https://secrets.example.com",
					Token:  "token",
					Expiry: float64(now.Add(10*time.Minute).UnixNano()) / 1e9,
				},
			},
			want:    10*time.Minute - sm.MaxScriptedTimeout,
			epsilon: defEpsilon,
		},
		"10 minute timeout, secret store expires in 1 hour": {
			timeout: 10 * time.Minute,
			tenant: &sm.Tenant{
				SecretStore: &sm.SecretStore{
					Url:    "https://secrets.example.com",
					Token:  "token",
					Expiry: float64(now.Add(1*time.Hour).UnixNano()) / 1e9,
				},
			},
			want:    10 * time.Minute,
			epsilon: defEpsilon,
		},
		"10 minute timeout, secret store with empty URL (not configured)": {
			timeout: 10 * time.Minute,
			tenant: &sm.Tenant{
				SecretStore: &sm.SecretStore{
					Url:    "",
					Token:  "token",
					Expiry: float64(now.Add(1*time.Hour).UnixNano()) / 1e9,
				},
			},
			want:    0, // Cache should be invalid immediately
			epsilon: defEpsilon,
		},
		"10 minute timeout, secret store with empty token (not configured)": {
			timeout: 10 * time.Minute,
			tenant: &sm.Tenant{
				SecretStore: &sm.SecretStore{
					Url:    "https://secrets.example.com",
					Token:  "",
					Expiry: float64(now.Add(1*time.Hour).UnixNano()) / 1e9,
				},
			},
			want:    0, // Cache should be invalid immediately
			epsilon: defEpsilon,
		},
		"10 minute timeout, secret store with both URL and token empty (not configured)": {
			timeout: 10 * time.Minute,
			tenant: &sm.Tenant{
				SecretStore: &sm.SecretStore{
					Url:    "",
					Token:  "",
					Expiry: float64(now.Add(1*time.Hour).UnixNano()) / 1e9,
				},
			},
			want:    0, // Cache should be invalid immediately
			epsilon: defEpsilon,
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			tm := &Manager{
				timeout: tc.timeout,
			}

			expected := time.Now().Add(tc.want)
			actual := tm.calculateValidUntil(tc.tenant)

			require.InEpsilonf(t, expected.UnixNano(), actual.UnixNano(), tc.epsilon,
				"calculateValidUntil() should be within range. expected: %d, actual: %d, delta: %d",
				expected.Sub(now).Milliseconds(),
				actual.Sub(now).Milliseconds(),
				expected.UnixMilli()-actual.UnixMilli(),
			)
		})
	}
}
