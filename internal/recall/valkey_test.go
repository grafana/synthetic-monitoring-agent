package recall_test

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/grafana/synthetic-monitoring-agent/internal/recall"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

func TestValkey(t *testing.T) {
	t.Parallel()

	redisEndpoint, redisPassword := startRedis(t)

	vr := recall.Valkey(recall.ValkeyOpts{
		Address:  redisEndpoint,
		Password: redisPassword,
	})

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	t.Run("returns nothing for a non existing value", func(t *testing.T) {
		t.Parallel()

		const checkId = 12345

		lastRun, err := vr.Recall(ctx, checkId)
		if err != nil {
			t.Fatalf("error recalling: %v", err)
		}

		zeroTime := time.Time{}

		if lastRun != zeroTime {
			t.Fatalf("expected zero time, got %v", lastRun)
		}
	})

	t.Run("returns stored time", func(t *testing.T) {
		t.Parallel()

		const checkId = 456789

		now := time.Now()
		err := vr.Remember(ctx, checkId, time.Hour)
		if err != nil {
			t.Fatalf("error remembering: %v", err)
		}

		lastRun, err := vr.Recall(ctx, checkId)
		if err != nil {
			t.Fatalf("error recalling: %v", err)
		}

		time.Sleep(time.Second)

		if now.Sub(lastRun).Abs() > 5*time.Second {
			t.Fatalf("expected lastRun to be close to %v, got %v", now, lastRun)
		}
	})

	t.Run("expires old runs", func(t *testing.T) {
		t.Parallel()

		const checkId = 112233

		err := vr.Remember(ctx, checkId, time.Second)
		if err != nil {
			t.Fatalf("error remembering: %v", err)
		}

		time.Sleep(2 * time.Second)

		lastRun, err := vr.Recall(ctx, checkId)
		if err != nil {
			t.Fatalf("error recalling: %v", err)
		}

		zeroTime := time.Time{}

		if lastRun != zeroTime {
			t.Fatalf("expected zero time, got %v", lastRun)
		}
	})
}

// startRedis starts a redis container for the given test context, and returns its address and its password.
// The container is registered to be destroyed on test cleanup.
func startRedis(t *testing.T) (string, string) {
	t.Helper()

	const redisPort = "6379/tcp"
	const redisPassword = "this is a test~!"

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	redis, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		Started: true,
		ContainerRequest: testcontainers.ContainerRequest{
			Image:        "redis:latest",
			ExposedPorts: []string{redisPort},
			Cmd: []string{
				"--requirepass",
				redisPassword,
			},
			WaitingFor: wait.ForAll(
				wait.ForLog("Ready to accept connections"),
				wait.ForExposedPort(),
			),
		},
	})
	if err != nil {
		t.Fatalf("creating redis container: %v", err)
	}

	t.Cleanup(func() {
		_ = redis.Terminate(ctx)
	})

	containerHost, err := redis.Host(ctx)
	if err != nil {
		t.Fatalf("obtaining redis container IP: %v", err)
	}

	mappedPort, err := redis.MappedPort(ctx, redisPort)
	if err != nil {
		t.Fatalf("obtaining redis mapped port: %v", err)
	}

	return net.JoinHostPort(containerHost, mappedPort.Port()), redisPassword
}
