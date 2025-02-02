package torrent

import (
	"container/heap"
	"fmt"
	"time"
	"unsafe"

	"github.com/anacrolix/multiless"
	"github.com/anacrolix/sync"
)

type worseConnInput struct {
	BadDirection        bool
	Useful              bool
	LastHelpful         time.Time
	CompletedHandshake  time.Time
	GetPeerPriority     func() (peerPriority, error)
	getPeerPriorityOnce sync.Once
	peerPriority        peerPriority
	peerPriorityErr     error
	Pointer             uintptr
}

func (me *worseConnInput) doGetPeerPriority() {
	me.peerPriority, me.peerPriorityErr = me.GetPeerPriority()
}

func (me *worseConnInput) doGetPeerPriorityOnce() {
	me.getPeerPriorityOnce.Do(me.doGetPeerPriority)
}

type worseConnLensOpts struct {
	incomingIsBad, outgoingIsBad bool
}

func worseConnInputFromPeer(p *Peer, opts worseConnLensOpts) worseConnInput {
	ret := worseConnInput{
		Useful:             p.useful(),
		LastHelpful:        p.lastHelpful(),
		CompletedHandshake: p.completedHandshake,
		Pointer:            uintptr(unsafe.Pointer(p)),
		GetPeerPriority:    p.peerPriority,
	}
	if opts.incomingIsBad && !p.outgoing {
		ret.BadDirection = true
	} else if opts.outgoingIsBad && p.outgoing {
		ret.BadDirection = true
	}
	return ret
}

func worseConn(_l, _r *Peer) bool {
	// TODO: Use generics for ptr to
	l := worseConnInputFromPeer(_l, worseConnLensOpts{})
	r := worseConnInputFromPeer(_r, worseConnLensOpts{})
	return l.Less(&r)
}

func (l *worseConnInput) Less(r *worseConnInput) bool {
	less, ok := multiless.New().Bool(
		r.BadDirection, l.BadDirection).Bool(
		l.Useful, r.Useful).CmpInt64(
		l.LastHelpful.Sub(r.LastHelpful).Nanoseconds()).CmpInt64(
		l.CompletedHandshake.Sub(r.CompletedHandshake).Nanoseconds()).LazySameLess(
		func() (same, less bool) {
			l.doGetPeerPriorityOnce()
			if l.peerPriorityErr != nil {
				same = true
				return
			}
			r.doGetPeerPriorityOnce()
			if r.peerPriorityErr != nil {
				same = true
				return
			}
			same = l.peerPriority == r.peerPriority
			less = l.peerPriority < r.peerPriority
			return
		}).Uintptr(
		l.Pointer, r.Pointer,
	).LessOk()
	if !ok {
		panic(fmt.Sprintf("cannot differentiate %#v and %#v", l, r))
	}
	return less
}

type worseConnSlice struct {
	conns []*PeerConn
	keys  []worseConnInput
}

func (me *worseConnSlice) initKeys(opts worseConnLensOpts) {
	me.keys = make([]worseConnInput, len(me.conns))
	for i, c := range me.conns {
		me.keys[i] = worseConnInputFromPeer(&c.Peer, opts)
	}
}

var _ heap.Interface = &worseConnSlice{}

func (me worseConnSlice) Len() int {
	return len(me.conns)
}

func (me worseConnSlice) Less(i, j int) bool {
	return me.keys[i].Less(&me.keys[j])
}

func (me *worseConnSlice) Pop() interface{} {
	i := len(me.conns) - 1
	ret := me.conns[i]
	me.conns = me.conns[:i]
	return ret
}

func (me *worseConnSlice) Push(x interface{}) {
	panic("not implemented")
}

func (me worseConnSlice) Swap(i, j int) {
	me.conns[i], me.conns[j] = me.conns[j], me.conns[i]
	me.keys[i], me.keys[j] = me.keys[j], me.keys[i]
}
