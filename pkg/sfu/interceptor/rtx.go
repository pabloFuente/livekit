package interceptor

import (
	"sync"

	"github.com/pion/interceptor"
	"github.com/pion/rtp"
	"github.com/pion/sdp/v3"
	"github.com/pion/webrtc/v3"

	"github.com/livekit/livekit-server/pkg/sfu/utils"
)

const (
	SDESRepairRTPStreamIDURI = "urn:ietf:params:rtp-hdrext:sdes:repaired-rtp-stream-id"
)

type streamInfo struct {
	ssrc uint32
	mid  string
	rid  string
	rsid string
}

type RTXInfoExtractorFactory struct {
	onRTXPairFound func(repair, base uint32)
	lock           sync.Mutex
	streams        []streamInfo
}

func (f *RTXInfoExtractorFactory) NewInterceptor(id string) (interceptor.Interceptor, error) {
}

func (f *RTXInfoExtractorFactory) setStreamInfo(ssrc uint32, mid, rid, rsid string) {
	f.lock.Lock()
	f.streams = append(f.streams, streamInfo{
		ssrc: ssrc,
		mid:  mid,
		rid:  rid,
		rsid: rsid,
	})

	if rsid != "" {
		// TODO-RTX: find corresponding base ssrc, delete...
	} else {
		// TODO-RTX: find corresponding rtx ssrc, delete...
	}
	f.lock.Unlock()
	f.onRTXPairFound(repair, base)
}

// TODO: handle RTX packets, also check if it is needed by migration
// inteceptor get rsid -> get ssrc of mid & rid -> bufferfactory.SetRTX
// or buffer get rsid -> get ssrc of mid & rid -> bufferfactory.SetRTX
type RTXInfoExtractor struct {
	interceptor.NoOp

	factory *RTXInfoExtractorFactory
	done    bool
}

func (u *RTXInfoExtractor) BindRemoteStream(info *interceptor.StreamInfo, reader interceptor.RTPReader) interceptor.RTPReader {
	midExtensionID := utils.GetHeaderExtensionID(info.RTPHeaderExtensions, webrtc.RTPHeaderExtensionCapability{URI: sdp.SDESMidURI})
	streamIDExtensionID := utils.GetHeaderExtensionID(info.RTPHeaderExtensions, webrtc.RTPHeaderExtensionCapability{URI: sdp.SDESRTPStreamIDURI})
	repairStreamIDExtensionID := utils.GetHeaderExtensionID(info.RTPHeaderExtensions, webrtc.RTPHeaderExtensionCapability{URI: SDESRepairRTPStreamIDURI})
	return interceptor.RTPReaderFunc(func(b []byte, attr interceptor.Attributes) (int, interceptor.Attributes, error) {
		n, a, err := reader.Read(b, attr)
		if u.done || err != nil {
			return n, a, err
		}
		var rp rtp.Packet
		if err1 := rp.Unmarshal(b); err1 != nil {
			return n, a, err
		}

		if !rp.Header.Extension {
			return n, a, err
		}

		var mid, rid, rsid string
		// payloadType = PayloadType(rp.PayloadType)
		if payload := rp.GetExtension(uint8(midExtensionID)); payload != nil {
			mid = string(payload)
		}

		if payload := rp.GetExtension(uint8(streamIDExtensionID)); payload != nil {
			rid = string(payload)
		}

		if payload := rp.GetExtension(uint8(repairStreamIDExtensionID)); payload != nil {
			rsid = string(payload)
		}

		if mid != "" && (rid != "" || rsid != "") {
			u.factory.setStreamInfo(rp.SSRC, mid, rid, rsid)
			u.done = true
		}
		return n, a, nil
	})
}
