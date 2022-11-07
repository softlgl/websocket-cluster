package handler

type ChannelData struct {
	Method string`json:"Method"`
	Group string`json:"Group"`
	MsgBody interface{}`json:"MsgBody"`
}