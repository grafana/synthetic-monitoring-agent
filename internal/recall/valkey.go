package recall

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

// ValkeyRecaller implements Recaller backed by a Valkey/Redict/Redis® database.
type ValkeyRecaller struct {
	opts      ValkeyOpts
	rdb       redis.UniversalClient
	clientMtx sync.Mutex
}

var _ Recaller = &ValkeyRecaller{} // Build-time assert ValkeyRecaller implements Recaller.

type ValkeyOpts struct {
	Address    string
	MasterName string
	Password   string
}

func Valkey(opts ValkeyOpts) *ValkeyRecaller {
	return &ValkeyRecaller{opts: opts}
}

const valkeyTimeFmt = time.RFC3339

func (v *ValkeyRecaller) Remember(ctx context.Context, globalCheckID int64, forgetAfter time.Duration) error {
	rdb, err := v.client(ctx)
	if err != nil {
		return fmt.Errorf("connecting to valkey: %w", err)
	}

	timestamp := time.Now().Format(valkeyTimeFmt)
	err = rdb.Set(ctx, v.key(globalCheckID), timestamp, forgetAfter).Err()
	if err != nil {
		// Fail this time, and flag the connection as borked.
		v.disconnect()
		return err
	}

	return nil
}

func (v *ValkeyRecaller) Recall(ctx context.Context, globalCheckID int64) (time.Time, error) {
	rdb, err := v.client(ctx)
	if err != nil {
		return time.Time{}, fmt.Errorf("connecting to valkey: %w", err)
	}

	lastRunStr, err := rdb.Get(ctx, v.key(globalCheckID)).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			// Key does not exist.
			return time.Time{}, nil
		}

		// Fail this time, and flag the connection as borked.
		v.disconnect()
		return time.Time{}, err
	}

	lastRun, err := time.Parse(valkeyTimeFmt, lastRunStr)
	if err != nil {
		return time.Time{}, fmt.Errorf("parsing time stored in valkey: %w", err)
	}

	return lastRun, nil
}

func (v *ValkeyRecaller) client(ctx context.Context) (redis.UniversalClient, error) {
	v.clientMtx.Lock()
	defer v.clientMtx.Unlock()

	if v.rdb != nil {
		return v.rdb, nil
	}

	rdb := redis.NewUniversalClient(&redis.UniversalOptions{
		Addrs:      []string{v.opts.Address},
		MasterName: v.opts.MasterName,
		Password:   v.opts.Password,
	})

	if err := rdb.Ping(ctx).Err(); err != nil {
		return nil, err
	}

	v.rdb = rdb
	return v.rdb, nil
}

func (v *ValkeyRecaller) disconnect() {
	v.clientMtx.Lock()
	defer v.clientMtx.Unlock()

	_ = v.rdb.Close()
	v.rdb = nil
}

func (_ *ValkeyRecaller) key(globalCheckID int64) string {
	return fmt.Sprintf("sm-agent:recall:%d", globalCheckID)
}
