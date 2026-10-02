package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// poolStatsTestChannel builds a channel of the given pool type with extra
// metadata, mirroring how the account pool stores per-account options.
func poolStatsTestChannel(id int, other string) *Channel {
	return &Channel{Id: id, Name: "pool", Type: 59, OtherInfo: other}
}

func TestPoolAcquireRelease(t *testing.T) {
	const id = 900001
	resetPoolStatsTestEntry(t, id)

	assert.True(t, PoolAcquire(id, 2))
	assert.True(t, PoolAcquire(id, 2))
	assert.False(t, PoolAcquire(id, 2), "third concurrent request must be refused")

	snapshot := PoolSnapshot(id)
	assert.Equal(t, 2, snapshot.ActiveRequests)

	PoolRelease(id)
	assert.Equal(t, 1, PoolSnapshot(id).ActiveRequests)
	PoolRelease(id)
	PoolRelease(id)
	assert.Equal(t, 0, PoolSnapshot(id).ActiveRequests, "release must stay idempotent at zero")
}

func TestPoolAcquireUnlimited(t *testing.T) {
	const id = 900002
	resetPoolStatsTestEntry(t, id)

	for i := 0; i < 8; i++ {
		assert.True(t, PoolAcquire(id, 0), "no ceiling means every request fits")
	}
	assert.Equal(t, 8, PoolSnapshot(id).ActiveRequests)
}

func TestPoolMarkResultErrorRate(t *testing.T) {
	const id = 900003
	resetPoolStatsTestEntry(t, id)

	assert.False(t, PoolIsOverloaded(id), "an account without samples is healthy")

	for i := 0; i < 3; i++ {
		PoolMarkResult(id, false)
	}
	assert.True(t, PoolIsOverloaded(id), "three consecutive failures cross the escape threshold")
	assert.EqualValues(t, 3, PoolSnapshot(id).TotalRequests)
	assert.EqualValues(t, 3, PoolSnapshot(id).TotalErrors)
	assert.InDelta(t, 0.58, PoolSnapshot(id).ErrorRate, 0.05)
	assert.NotZero(t, PoolSnapshot(id).LastUsedAt)

	// Recoveries pull the EWMA back down below the threshold.
	for i := 0; i < 12; i++ {
		PoolMarkResult(id, true)
	}
	assert.False(t, PoolIsOverloaded(id))
}

func TestPoolReapLeaked(t *testing.T) {
	const id = 900004
	resetPoolStatsTestEntry(t, id)

	PoolForceOccupy(id)
	PoolForceOccupy(id)
	assert.Equal(t, 2, PoolSnapshot(id).ActiveRequests)

	// lastActiveAt is "now", so a large idle window reaps nothing until the
	// entry actually ages; force-aging keeps the test deterministic.
	poolStatsMu.Lock()
	poolStatsByID[id].lastActiveAt -= 3600
	poolStatsMu.Unlock()

	assert.Equal(t, 1, PoolReapLeaked(600))
	assert.Equal(t, 0, PoolSnapshot(id).ActiveRequests)
}

func TestPoolConcurrencyLimit(t *testing.T) {
	assert.Equal(t, 0, PoolConcurrencyLimit(nil))
	assert.Equal(t, 0, PoolConcurrencyLimit(&Channel{Id: 1, OtherInfo: ""}))
	assert.Equal(t, 0, PoolConcurrencyLimit(poolStatsTestChannel(1, `{"priority":50}`)))
	assert.Equal(t, 4, PoolConcurrencyLimit(poolStatsTestChannel(1, `{"pool_max_concurrency":4}`)))
	assert.Equal(t, 4, PoolConcurrencyLimit(poolStatsTestChannel(1, `{"pool_max_concurrency":"4"}`)))
	assert.Equal(t, 0, PoolConcurrencyLimit(poolStatsTestChannel(1, `{"pool_max_concurrency":"x"}`)))
	assert.Equal(t, 1024, PoolConcurrencyLimit(poolStatsTestChannel(1, `{"pool_max_concurrency":4096}`)))
}

func TestPoolAwareOrderChannelsKeepsHealthyTopK(t *testing.T) {
	const hot = 900010
	const cold = 900011
	const off = 900012
	for _, id := range []int{hot, cold, off} {
		resetPoolStatsTestEntry(t, id)
	}

	inputs := []*Channel{
		poolStatsTestChannel(hot, `{"pool_max_concurrency":1}`),
		poolStatsTestChannel(cold, `{"pool_max_concurrency":1}`),
		poolStatsTestChannel(off, `{"pool_max_concurrency":1}`),
	}

	// hot has a failing EWMA, cold runs at its concurrency ceiling and is
	// stale, off is completely clean: off must rank first.
	poolStatsMu.Lock()
	poolStatsByID[hot].samples = 5
	poolStatsByID[hot].errEWMA = 0.9
	poolStatsByID[cold].active = 1
	poolStatsByID[cold].lastActiveAt = 1
	poolStatsByID[off].lastActiveAt = 100
	poolStatsMu.Unlock()

	ordered := PoolAwareOrderChannels(inputs)
	assert.Len(t, ordered, 3)
	assert.Equal(t, off, ordered[0].Id, "the healthiest account ranks first")
	assert.Contains(t, []int{hot, cold}, ordered[1].Id)
}

func TestPoolAwareOrderChannelsNonPoolUnaffected(t *testing.T) {
	ordinary := []*Channel{
		{Id: 700001, Type: 1},
		{Id: 700002, Type: 14},
	}
	ordered := PoolAwareOrderChannels(ordinary)
	assert.Equal(t, ordinary, ordered, "non-pool channels keep their exact order")
}

func TestPoolIsSaturated(t *testing.T) {
	const id = 900013
	resetPoolStatsTestEntry(t, id)

	channel := poolStatsTestChannel(id, `{"pool_max_concurrency":1}`)
	assert.False(t, PoolIsSaturated(channel))
	PoolForceOccupy(id)
	assert.True(t, PoolIsSaturated(channel))
	PoolRelease(id)
	assert.False(t, PoolIsSaturated(channel))
}

func resetPoolStatsTestEntry(t *testing.T, id int) {
	t.Helper()
	poolStatsMu.Lock()
	poolStatsByID[id] = &poolAccountStats{}
	poolStatsMu.Unlock()
	t.Cleanup(func() {
		poolStatsMu.Lock()
		delete(poolStatsByID, id)
		poolStatsMu.Unlock()
	})
}
