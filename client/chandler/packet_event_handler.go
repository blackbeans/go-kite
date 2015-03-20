package chandler

import (
	"errors"
	. "kiteq/pipe"
	"kiteq/protocol"

	// "log"
)

//远程操作的PacketHandler

type PacketHandler struct {
	BaseForwardHandler
}

func NewPacketHandler(name string) *PacketHandler {
	packetHandler := &PacketHandler{}
	packetHandler.BaseForwardHandler = NewBaseForwardHandler(name, packetHandler)
	return packetHandler

}

func (self *PacketHandler) TypeAssert(event IEvent) bool {
	_, ok := self.cast(event)
	return ok
}

func (self *PacketHandler) cast(event IEvent) (val *PacketEvent, ok bool) {
	val, ok = event.(*PacketEvent)

	return
}

var INVALID_PACKET_ERROR = errors.New("INVALID PACKET ERROR")

func (self *PacketHandler) Process(ctx *DefaultPipelineContext, event IEvent) error {

	// log.Printf("PacketHandler|Process|%s|%t\n", self.GetName(), event)

	pevent, ok := self.cast(event)
	if !ok {
		return ERROR_INVALID_EVENT_TYPE
	}

	cevent, err := self.handlePacket(pevent)
	if nil != err {
		return err
	}
	ctx.SendForward(cevent)
	return nil
}

var eventSunk = &SunkEvent{}

//对于请求事件
func (self *PacketHandler) handlePacket(pevent *PacketEvent) (IEvent, error) {
	var err error
	var event IEvent
	packet := pevent.Packet
	//根据类型反解packet
	switch packet.CmdType {
	//连接授权确认
	case protocol.CMD_CONN_AUTH:
		var auth protocol.ConnAuthAck
		err = protocol.PbMarshaler.UnmarshalMessage(packet.Data, &auth)
		if nil == err {
			pevent.RemoteClient.Attach(packet.Opaque, &auth)
			event = &SunkEvent{}
		}

	//心跳
	case protocol.CMD_HEARTBEAT:
		var hearbeat protocol.HeartBeat
		err = protocol.PbMarshaler.UnmarshalMessage(packet.Data, &hearbeat)
		if nil == err {
			hb := &hearbeat
			// log.Printf("PacketHandler|handlePacket|HeartBeat|%t\n", hb)
			event = NewHeartbeatEvent(pevent.RemoteClient, packet.Opaque, hb.GetVersion())
		}
		//消息持久化
	case protocol.CMD_MESSAGE_STORE_ACK:
		var pesisteAck protocol.MessageStoreAck
		err = protocol.PbMarshaler.UnmarshalMessage(packet.Data, &pesisteAck)
		if nil == err {
			//直接通知当前的client对应chan
			pevent.RemoteClient.Attach(packet.Opaque, &pesisteAck)
			event = eventSunk
		}

	case protocol.CMD_TX_ACK:
		var txAck protocol.TxACKPacket
		err = protocol.PbMarshaler.UnmarshalMessage(packet.Data, &txAck)
		if nil == err {
			event = newAcceptEvent(protocol.CMD_TX_ACK, &txAck, pevent.RemoteClient, packet.Opaque)
		}
	//发送的是bytesmessage
	case protocol.CMD_BYTES_MESSAGE:
		var msg protocol.BytesMessage
		err = protocol.PbMarshaler.UnmarshalMessage(packet.Data, &msg)
		if nil == err {
			event = newAcceptEvent(protocol.CMD_BYTES_MESSAGE, &msg, pevent.RemoteClient, packet.Opaque)
		}
	//发送的是StringMessage
	case protocol.CMD_STRING_MESSAGE:
		var msg protocol.StringMessage
		err = protocol.PbMarshaler.UnmarshalMessage(packet.Data, &msg)
		if nil == err {
			event = newAcceptEvent(protocol.CMD_STRING_MESSAGE, &msg, pevent.RemoteClient, packet.Opaque)
		}
	}

	return event, err

}
