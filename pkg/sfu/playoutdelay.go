package sfu

import (
	"sync"
	"sync/atomic"

	"github.com/livekit/livekit-server/pkg/sfu/rtpextension"
	"github.com/livekit/protocol/logger"
)

const (
	PlayoutDelayStateChanged int32 = iota
	PlayoutDelaySending
	PlayoutDelayAcked

	jitterMultiToDelay = 25
)

type PlayoutDelayController struct {
	lock               sync.Mutex
	state              atomic.Int32
	minDelay, maxDelay uint32
	currentDelay       uint32
	extBytes           atomic.Value //[]byte
	sendingAtSeq       uint16
	logger             logger.Logger
}

func NewPlayoutDelayController(minDelay, maxDelay uint32, logger logger.Logger) (*PlayoutDelayController, error) {
	if maxDelay == 0 || maxDelay > rtpextension.PlayoutDelayDefaultMax {
		maxDelay = rtpextension.PlayoutDelayDefaultMax
	}
	c := &PlayoutDelayController{
		currentDelay: minDelay,
		minDelay:     minDelay,
		maxDelay:     maxDelay,
		logger:       logger,
	}
	return c, c.createExtData()
}

func (c *PlayoutDelayController) SetJitter(jitter uint32) {
	c.lock.Lock()
	targetDelay := jitter * jitterMultiToDelay
	// increase delay quickly, decrease slowly to make fps more stable
	if targetDelay > c.currentDelay {
		targetDelay = (targetDelay-c.currentDelay)*3/4 + c.currentDelay
	} else {
		targetDelay = c.currentDelay - (c.currentDelay-targetDelay)/5
	}
	if targetDelay < c.minDelay {
		targetDelay = c.minDelay
	}
	if targetDelay > c.maxDelay {
		targetDelay = c.maxDelay
	}
	if c.currentDelay == targetDelay {
		c.lock.Unlock()
		return
	}
	c.currentDelay = targetDelay
	c.lock.Unlock()
	c.createExtData()
}

func (c *PlayoutDelayController) OnSeqAcked(seq uint16) {
	c.lock.Lock()
	defer c.lock.Unlock()
	if c.state.Load() == PlayoutDelaySending && (seq-c.sendingAtSeq) < 0x8000 {
		c.state.Store(PlayoutDelayAcked)
	}
}

func (c *PlayoutDelayController) GetDelayExtension(seq uint16) []byte {
	switch c.state.Load() {
	case PlayoutDelayStateChanged:
		c.lock.Lock()
		c.state.Store(PlayoutDelaySending)
		c.sendingAtSeq = seq
		c.lock.Unlock()
		return c.extBytes.Load().([]byte)
	case PlayoutDelaySending:
		return c.extBytes.Load().([]byte)
	case PlayoutDelayAcked:
		return nil
	}
	return nil
}

func (c *PlayoutDelayController) createExtData() error {
	delay := rtpextension.PlayoutDelayFromValue(
		uint16(c.currentDelay),
		uint16(c.maxDelay),
	)
	b, err := delay.Marshal()
	if err == nil {
		c.extBytes.Store(b)
		c.state.Store(int32(PlayoutDelayStateChanged))
	} else {
		c.logger.Errorw("failed to marshal playout delay", err, "playoutDelay", delay)
	}
	return err
}
