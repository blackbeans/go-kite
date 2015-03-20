package handler

import (
	. "kiteq/pipe"
	"kiteq/protocol"
	// "log"
)

type HeartbeatHandler struct {
	BaseForwardHandler
}

//------创建heartbeat
func NewHeartbeatHandler(name string) *HeartbeatHandler {
	phandler := &HeartbeatHandler{}
	phandler.BaseForwardHandler = NewBaseForwardHandler(name, phandler)
	return phandler
}

func (self *HeartbeatHandler) TypeAssert(event IEvent) bool {
	_, ok := self.cast(event)
	return ok
}

func (self *HeartbeatHandler) cast(event IEvent) (val *HeartbeatEvent, ok bool) {
	val, ok = event.(*HeartbeatEvent)
	return
}

func (self *HeartbeatHandler) Process(ctx *DefaultPipelineContext, event IEvent) error {

	hevent, ok := self.cast(event)
	if !ok {
		return ERROR_INVALID_EVENT_TYPE
	}

	//处理本地的pong
	hevent.RemoteClient.Pong(hevent.Opaque, hevent.Version)
	var packet *protocol.Packet
	packet = protocol.NewRespPacket(hevent.Opaque, protocol.CMD_HEARTBEAT, hevent.RemoteClient.Marshaler.MarshalHeartbeatPacket(hevent.Version))
	//发起一个网络请求
	remoteEvent := NewRemotingEvent(packet, []string{hevent.RemoteClient.RemoteAddr()})

	// log.Printf("HeartbeatHandler|%s|Process|Recieve|Ping|%s|%d\n", self.GetName(), hevent.RemoteClient.RemoteAddr(), hevent.Version)
	ctx.SendForward(remoteEvent)
	return nil
}
