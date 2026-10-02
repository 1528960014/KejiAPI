package model

import (
	"sync"
	"time"

	"kejiapi/constant"
	"kejiapi/setting/operation_setting"
)

// PoolAccountStatsSnapshot is the live per-account runtime view of a pooled
// subscription / gateway account. It powers the account pool table in the
// console and drives pool-aware scheduling (hot accounts lose traffic).
type PoolAccountStatsSnapshot struct {
	ActiveRequests int     `json:"active_requests"`
	TotalRequests  int64   `json:"total_requests"`
	TotalErrors    int64   `json:"total_errors"`
	ErrorRate      float64 `json:"error_rate"`
	LastUsedAt     int64   `json:"last_used_at"`
}

type poolAccountStats struct {
	active       int64
	total        int64
	errors       int64
	samples      int64
	errEWMA      float64
	lastUsedAt   int64
	lastActiveAt int64
}

const (
	// poolStatsEWMAAlpha is the smoothing factor of the per-account error
	// rate sliding average: recent outcomes matter more than old ones.
	poolStatsEWMAAlpha = 0.25
	// poolStatsEscapeSamples is the number of recorded results before the
	// error-rate escape heuristics trust the EWMA value.
	poolStatsEscapeSamples = 3
	// poolStatsReapAfterSeconds clears the active counter of accounts whose
	// last activity is older than this, so transport paths without an
	// explicit release (long-lived websocket relays) cannot leak slots
	// forever.
	poolStatsReapAfterSeconds = 600
	// poolStatsMaxEntries caps the in-memory table; accounts that have not
	// been active recently are dropped when the cap is exceeded.
	poolStatsMaxEntries = 4096
)

var (
	poolStatsMu   sync.Mutex
	poolStatsByID = make(map[int]*poolAccountStats)
)

// poolStatsEntryLocked returns (creating if needed) the stats record. The
// caller must hold poolStatsMu.
func poolStatsEntryLocked(id int) *poolAccountStats {
	stats, ok := poolStatsByID[id]
	if !ok {
		stats = &poolAccountStats{}
		poolStatsByID[id] = stats
	}
	return stats
}

// PoolAcquire occupies one concurrency slot of a pooled account. It returns
// false when the account reached its per-account concurrency ceiling
// (maxConcurrency <= 0 means "no ceiling"). Callers that must not fail the
// request can degrade with PoolForceOccupy.
func PoolAcquire(channelID int, maxConcurrency int) bool {
	if channelID <= 0 {
		return true
	}
	poolStatsMu.Lock()
	defer poolStatsMu.Unlock()
	stats := poolStatsEntryLocked(channelID)
	stats.lastActiveAt = time.Now().Unix()
	if maxConcurrency > 0 && stats.active >= int64(maxConcurrency) {
		return false
	}
	stats.active++
	return true
}

// PoolForceOccupy records one in-flight request against a pooled account
// without enforcing the concurrency ceiling (soft degradation).
func PoolForceOccupy(channelID int) {
	if channelID <= 0 {
		return
	}
	poolStatsMu.Lock()
	defer poolStatsMu.Unlock()
	stats := poolStatsEntryLocked(channelID)
	stats.active++
	stats.lastActiveAt = time.Now().Unix()
}

// PoolRelease frees one concurrency slot previously occupied by
// PoolAcquire / PoolForceOccupy.
func PoolRelease(channelID int) {
	if channelID <= 0 {
		return
	}
	poolStatsMu.Lock()
	defer poolStatsMu.Unlock()
	stats, ok := poolStatsByID[channelID]
	if !ok {
		return
	}
	if stats.active > 0 {
		stats.active--
	}
	stats.lastActiveAt = time.Now().Unix()
}

// PoolMarkResult reports the outcome of one served request. Failures feed
// the per-account error-rate sliding average used by the scheduler to move
// traffic away from unhealthy accounts.
func PoolMarkResult(channelID int, success bool) {
	if channelID <= 0 {
		return
	}
	poolStatsMu.Lock()
	defer poolStatsMu.Unlock()
	stats := poolStatsEntryLocked(channelID)
	now := time.Now().Unix()
	stats.total++
	stats.samples++
	stats.lastUsedAt = now
	stats.lastActiveAt = now
	if !success {
		stats.errors++
	}
	fail := 0.0
	if !success {
		fail = 1.0
	}
	stats.errEWMA = (1-poolStatsEWMAAlpha)*stats.errEWMA + poolStatsEWMAAlpha*fail
}

// PoolSnapshot returns a copy of the live runtime view of one account.
func PoolSnapshot(channelID int) PoolAccountStatsSnapshot {
	poolStatsMu.Lock()
	defer poolStatsMu.Unlock()
	stats, ok := poolStatsByID[channelID]
	if !ok {
		return PoolAccountStatsSnapshot{}
	}
	return PoolAccountStatsSnapshot{
		ActiveRequests: int(stats.active),
		TotalRequests:  stats.total,
		TotalErrors:    stats.errors,
		ErrorRate:      stats.errEWMA,
		LastUsedAt:     stats.lastUsedAt,
	}
}

// PoolErrorRateEscapeThreshold is the EWMA error rate at or above which the
// scheduler treats a pooled account as unhealthy and stops preferring it.
// Three consecutive failures push the EWMA (alpha 0.25) to 0.58, so the
// threshold sits just below that: a short burst of errors is enough to
// escape a sticky account.
func PoolErrorRateEscapeThreshold() float64 {
	return 0.5
}

// PoolIsOverloaded reports whether the account's recent error rate is high
// enough to be skipped by session-sticky routing.
func PoolIsOverloaded(channelID int) bool {
	poolStatsMu.Lock()
	defer poolStatsMu.Unlock()
	stats, ok := poolStatsByID[channelID]
	if !ok {
		return false
	}
	if stats.samples < poolStatsEscapeSamples {
		return false
	}
	return stats.errEWMA >= PoolErrorRateEscapeThreshold()
}

// PoolReapLeaked clears the active counter of accounts that have been idle
// longer than maxIdleSeconds. It exists for transport paths that never run
// a release callback. Returns the number of accounts reaped.
func PoolReapLeaked(maxIdleSeconds int64) int {
	if maxIdleSeconds <= 0 {
		maxIdleSeconds = poolStatsReapAfterSeconds
	}
	poolStatsMu.Lock()
	defer poolStatsMu.Unlock()
	now := time.Now().Unix()
	reaped := 0
	for _, stats := range poolStatsByID {
		if stats.active <= 0 {
			continue
		}
		if now-stats.lastActiveAt >= maxIdleSeconds {
			stats.active = 0
			reaped++
		}
	}
	return reaped
}

// PoolConcurrencyLimit reads the per-account concurrency ceiling from the
// channel's extra metadata ("pool_max_concurrency"). 0 means unlimited.
// The metadata lives in the channel cache, so parsing it here is cheap
// relative to one upstream request and keeps selection lock-free.
func PoolConcurrencyLimit(channel *Channel) int {
	if channel == nil || channel.OtherInfo == "" {
		return 0
	}
	value, ok := channel.GetOtherInfo()["pool_max_concurrency"]
	if !ok {
		return 0
	}
	limit := 0
	switch v := value.(type) {
	case float64:
		limit = int(v)
	case int:
		limit = v
	case int64:
		limit = int(v)
	case string:
		parsed := 0
		for _, r := range v {
			if r < '0' || r > '9' {
				parsed = -1
				break
			}
			parsed = parsed*10 + int(r-'0')
		}
		if parsed > 0 {
			limit = parsed
		}
	}
	if limit < 0 {
		limit = 0
	}
	if limit > 1024 {
		limit = 1024
	}
	return limit
}

// PoolSelectionTopK is the number of healthiest pool accounts that take part
// in each weighted draw. 0 or less disables load-aware ordering.
func PoolSelectionTopK() int {
	setting := operation_setting.GetAccountPoolSetting()
	if setting.SelectionTopK > 0 {
		return setting.SelectionTopK
	}
	return 3
}

// PoolAwareOrderChannels reorders candidate channels for the weighted draw:
// pooled accounts are ranked by live health (overloaded first, then active
// load, then error rate, then staleness) and only the top-K survive, so a
// hot or failing account stops absorbing traffic. Non-pool channels keep
// their relative order and are appended first, so ordinary channels are
// completely unaffected by pool scheduling.
func PoolAwareOrderChannels(channels []*Channel) []*Channel {
	if len(channels) <= 1 {
		return channels
	}
	pooled := make([]*Channel, 0, len(channels))
	for _, channel := range channels {
		if channel == nil {
			continue
		}
		if constant.IsSubscriptionPoolChannelType(channel.Type) {
			pooled = append(pooled, channel)
		}
	}
	if len(pooled) <= 1 {
		return channels
	}
	topK := PoolSelectionTopK()
	if topK <= 0 {
		return channels
	}

	ranked := make([]*Channel, len(pooled))
	copy(ranked, pooled)
	for i := 1; i < len(ranked); i++ {
		for j := i; j > 0 && poolRankLess(ranked[j], ranked[j-1]); j-- {
			ranked[j], ranked[j-1] = ranked[j-1], ranked[j]
		}
	}
	if len(ranked) > topK {
		ranked = ranked[:topK]
	}

	others := make([]*Channel, 0, len(channels))
	kept := make(map[int]bool, len(ranked))
	for _, channel := range ranked {
		kept[channel.Id] = true
	}
	for _, channel := range channels {
		if channel == nil || kept[channel.Id] {
			continue
		}
		others = append(others, channel)
	}
	return append(others, ranked...)
}

// poolRankLess reports whether channel a should be drawn before channel b.
func poolRankLess(a, b *Channel) bool {
	ra, rb := poolRankTuple(a, b)
	for i := range ra {
		if ra[i] != rb[i] {
			return ra[i] < rb[i]
		}
	}
	return a.Id < b.Id
}

func poolRankTuple(a, b *Channel) ([]int64, []int64) {
	return []int64{
		poolOverloadRank(a),
		int64(PoolSnapshot(a.Id).ActiveRequests),
		poolErrorRank(a),
		-PoolSnapshot(a.Id).LastUsedAt,
	}, []int64{
		poolOverloadRank(b),
		int64(PoolSnapshot(b.Id).ActiveRequests),
		poolErrorRank(b),
		-PoolSnapshot(b.Id).LastUsedAt,
	}
}

func poolOverloadRank(channel *Channel) int64 {
	limit := PoolConcurrencyLimit(channel)
	if limit <= 0 {
		return 0
	}
	if int64(limit) <= int64(PoolSnapshot(channel.Id).ActiveRequests) {
		return 1
	}
	return 0
}

func poolErrorRank(channel *Channel) int64 {
	poolStatsMu.Lock()
	stats, ok := poolStatsByID[channel.Id]
	if !ok {
		poolStatsMu.Unlock()
		return 0
	}
	samples := stats.samples
	ewma := stats.errEWMA
	poolStatsMu.Unlock()
	if samples < poolStatsEscapeSamples || ewma < PoolErrorRateEscapeThreshold() {
		return 0
	}
	return 1
}

// PoolIsSaturated reports whether a pooled account already runs at its
// per-account concurrency ceiling. Session-sticky routing uses it to
// fall through to load-balanced selection instead of queueing behind the
// pinned account.
func PoolIsSaturated(channel *Channel) bool {
	if channel == nil {
		return false
	}
	limit := PoolConcurrencyLimit(channel)
	if limit <= 0 {
		return false
	}
	return PoolSnapshot(channel.Id).ActiveRequests >= limit
}
