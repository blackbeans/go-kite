package handler

import (
	"errors"
	. "kiteq/pipe"
	"kiteq/protocol"
	"kiteq/stat"
	"kiteq/store"
	// "log"
	"sort"
)

var ERROR_PERSISTENT = errors.New("persistent msg error!")

//----------------持久化的handler
type PersistentHandler struct {
	BaseForwardHandler
	kitestore     store.IKiteStore
	maxDeliverNum chan byte
	topics        []string
	flowstat      *stat.FlowStat
}

//------创建persitehandler
func NewPersistentHandler(name string, flowstat *stat.FlowStat, topics []string, maxDeliverWorker int, kitestore store.IKiteStore) *PersistentHandler {
	phandler := &PersistentHandler{}
	phandler.BaseForwardHandler = NewBaseForwardHandler(name, phandler)
	phandler.kitestore = kitestore
	phandler.maxDeliverNum = make(chan byte, maxDeliverWorker)
	sort.Strings(topics)
	phandler.topics = topics
	phandler.flowstat = flowstat
	return phandler
}

func (self *PersistentHandler) TypeAssert(event IEvent) bool {
	_, ok := self.cast(event)
	return ok
}

func (self *PersistentHandler) cast(event IEvent) (val *persistentEvent, ok bool) {
	val, ok = event.(*persistentEvent)
	return
}

func (self *PersistentHandler) Process(ctx *DefaultPipelineContext, event IEvent) error {

	pevent, ok := self.cast(event)
	if !ok {
		return ERROR_INVALID_EVENT_TYPE
	}

	if nil != pevent.entity {

		//先判断是否是可以处理的topic的消息
		idx := sort.SearchStrings(self.topics, pevent.entity.Header.GetTopic())
		if idx == len(self.topics) {
			//不存在该消息的处理则直接返回存储失败
			remoteEvent := NewRemotingEvent(self.storeAck(pevent.opaque,
				pevent.entity.Header.GetMessageId(), false, "UnSupport Topic Message!", pevent.remoteClient.Marshaler),
				[]string{pevent.remoteClient.RemoteAddr()})
			ctx.SendForward(remoteEvent)
		} else {

			//如果是fly模式不做持久化
			if pevent.entity.Header.GetFly() {
				if pevent.entity.Header.GetCommit() {
					//如果是成功存储的、并且为未提交的消息，则需要发起一个ack的命令
					//发送存储结果ack
					remoteEvent := NewRemotingEvent(self.storeAck(pevent.opaque,
						pevent.entity.Header.GetMessageId(), true, "FLY NO NEED SAVE", pevent.remoteClient.Marshaler), []string{pevent.remoteClient.RemoteAddr()})
					ctx.SendForward(remoteEvent)
					self.send(ctx, pevent)
				} else {
					remoteEvent := NewRemotingEvent(self.storeAck(pevent.opaque,
						pevent.entity.Header.GetMessageId(), false, "FLY MUST BE COMMITTED !", pevent.remoteClient.Marshaler), []string{pevent.remoteClient.RemoteAddr()})
					ctx.SendForward(remoteEvent)
				}

			} else {
				self.sendUnFlyMessage(ctx, pevent)
			}
		}
	}

	return nil
}

//发送非flymessage
func (self *PersistentHandler) sendUnFlyMessage(ctx *DefaultPipelineContext, pevent *persistentEvent) {

	//写入到持久化存储里面
	succ := self.kitestore.Save(pevent.entity)
	//发送存储结果ack
	remoteEvent := NewRemotingEvent(self.storeAck(pevent.opaque,
		pevent.entity.Header.GetMessageId(), succ, "", pevent.remoteClient.Marshaler), []string{pevent.remoteClient.RemoteAddr()})
	ctx.SendForward(remoteEvent)

	//如果是commit的消息先尝试投递、再做持久化
	if succ && pevent.entity.Header.GetCommit() {
		self.send(ctx, pevent)
	}

	//如果是成功存储的、并且为未提交的消息，则需要发起一个ack的命令
	if succ && !pevent.entity.Header.GetCommit() {

		remoteEvent := NewRemotingEvent(self.tXAck(
			pevent.entity.Header, pevent.remoteClient.Marshaler), []string{pevent.remoteClient.RemoteAddr()})
		ctx.SendForward(remoteEvent)
	}
}

func (self *PersistentHandler) send(ctx *DefaultPipelineContext, pevent *persistentEvent) {

	f := func() {
		//启动投递当然会重投3次
		deliver := NewDeliverPreEvent(
			pevent.entity.Header.GetMessageId(),
			pevent.entity.Header,
			pevent.entity)
		ctx.SendForward(deliver)
		self.flowstat.DeliverFlow.Incr(1)
	}

	self.maxDeliverNum <- 1
	self.flowstat.DeliverPool.Incr(1)
	go func() {
		defer func() {
			<-self.maxDeliverNum
			self.flowstat.DeliverPool.Incr(-1)
		}()
		//启动投递
		f()
	}()

	// log.Println("PersistentHandler|send|FULL|TRY SEND BY CURRENT GO ....")
}

func (self *PersistentHandler) storeAck(opaque int32, messageid string, succ bool, feedback string, marshaler protocol.MarshalHelper) *protocol.Packet {
	var storeAck []byte
	storeAck = marshaler.MarshalMessageStoreAck(messageid, succ, feedback)
	//响应包
	return protocol.NewRespPacket(opaque, protocol.CMD_MESSAGE_STORE_ACK, storeAck)

}

//发送事务ack信息
func (self *PersistentHandler) tXAck(
	header *protocol.Header, marshaler protocol.MarshalHelper) *protocol.Packet {
	var txack []byte
	txack = marshaler.MarshalTxACKPacket(header, protocol.TX_UNKNOWN, "Server Check")
	//响应包
	return protocol.NewPacket(protocol.CMD_TX_ACK, txack)

}
