package discovery

import (
	"context"
	"errors"
	"sync"

	"github.com/libp2p/go-libp2p-core/peer"
)

// limitedSet is a thread safe set of peers with given limit.
// Inspired by libp2p peer.Set but extended with Remove method.
type limitedSet struct {
	lk sync.RWMutex
	ps map[peer.ID]struct{}

	limit    uint
	waitPeer chan peer.ID
}

// newLimitedSet constructs a set with the maximum peers amount.
func newLimitedSet(limit uint) *limitedSet {
	ps := new(limitedSet)
	ps.ps = make(map[peer.ID]struct{})
	ps.limit = limit
	ps.waitPeer = make(chan peer.ID)
	return ps
}

func (ps *limitedSet) Contains(p peer.ID) bool {
	ps.lk.RLock()
	_, ok := ps.ps[p]
	ps.lk.RUnlock()
	return ok
}

func (ps *limitedSet) Size() int {
	ps.lk.RLock()
	defer ps.lk.RUnlock()
	return len(ps.ps)
}

// TryAdd attempts to add the given peer into the set.
// This operation will fail if the number of peers in the set is equal to size.
func (ps *limitedSet) TryAdd(p peer.ID) error {
	ps.lk.Lock()
	defer ps.lk.Unlock()
	if _, ok := ps.ps[p]; ok {
		return errors.New("share: discovery: peer already added")
	}
	if len(ps.ps) >= int(ps.limit) {
		return errors.New("share: discovery: peers limit reached")
	}

	ps.ps[p] = struct{}{}
LOOP:
	for {
		// peer will be pushed to the channel only when somebody is reading from it.
		// this is done to handle case when Peers() was called on empty set.
		select {
		case ps.waitPeer <- p:
		default:
			break LOOP
		}
	}
	return nil
}

func (ps *limitedSet) Remove(id peer.ID) {
	ps.lk.Lock()
	defer ps.lk.Unlock()
	if ps.limit > 0 {
		delete(ps.ps, id)
	}
}

// Peers returns all discovered peers from the set.
func (ps *limitedSet) Peers(ctx context.Context) ([]peer.ID, error) {
	ps.lk.Lock()
	if len(ps.ps) > 0 {
		out := make([]peer.ID, 0, len(ps.ps))
		for p := range ps.ps {
			out = append(out, p)
		}
		ps.lk.Unlock()
		return out, nil
	}
	ps.lk.Unlock()

	// block until a new peer will be discovered
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case p := <-ps.waitPeer:
		return []peer.ID{p}, nil
	}
}
