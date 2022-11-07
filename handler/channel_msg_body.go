package handler

type ChannelMsgBody struct {
	FromId string`json:"FromId"`
	ToId string`json:"ToId"`
	Msg string`json:"Msg"`
}
