package handler

import (
	"encoding/json"
	"fmt"
	"github.com/go-redis/redis/v9"
	"golang.org/x/net/context"
	"golang.org/x/net/websocket"
	"log"
	"os"
	"strings"
	"sync"
)

var users map[string]*websocket.Conn
var groupUser map[string]map[string]byte
var subs map[string]*redis.PubSub

var ctx = context.Background()
var rdb *redis.Client

var groupLock sync.RWMutex
var joinLock sync.Mutex
var subLock sync.Mutex

const (
	userPrefix = "user:"
	groupPrefix = "group:"
	all = "all"
)

func HandleChannel(ws *websocket.Conn)  {
	strArr := strings.Split(ws.Request().RequestURI,"/")
	var userId = strArr[len(strArr)-1]
	log.Printf("user:%s-%s join", userId, ws.Request().RemoteAddr)

	var wg sync.WaitGroup
	joinLock.Lock()

	users[userId] = ws
	wg.Add(1)
	go subMsg(userPrefix+ userId, &wg)
	if len(users)==1 {
		wg.Add(1)
		go subAllMsg(all, &wg)
	}
	wg.Wait()

	joinLock.Unlock()

	for {
		var reply string
		if recErr := websocket.Message.Receive(ws, &reply); recErr != nil {
			log.Printf("user %s can't receive", userId)
			break
		}

		channelData := ChannelData{}
		_ = json.Unmarshal([]byte(reply), &channelData)

		switch channelData.Method {
		case "One":
			msgBytes, _ := json.Marshal(channelData.MsgBody)
			go handleOne(userId, msgBytes)
		case "UserGroup":
			go addUserGroup(userId, channelData.Group, ws)
		case "Group":
			go handleGroup(channelData.Group, userId, ws, channelData.MsgBody.(string))
		default:
			go handleAll(userId, channelData.MsgBody)
		}
	}

	joinLock.Lock()
	delete(users, userId)
	joinLock.Unlock()

	subLock.Lock()

	sub := subs[userPrefix+ userId]
	_ = sub.Close()
	delete(subs, userPrefix+ userId)

	subLock.Unlock()
}

func handleOne(userId string, msg []byte){
	msgBody := MsgBody{}
	_ = json.Unmarshal(msg, &msgBody)
	if targetWs, ok := users[msgBody.Id]; ok{
		res := fmt.Sprintf("user %s send:%s", userId, msgBody.Msg)
		if sendErr := websocket.Message.Send(targetWs, res); sendErr != nil {
			log.Printf("can't send [%s] to user %s", msgBody.Msg, msgBody.Id)
			return
		}
	} else {
		chMsgBody:= ChannelMsgBody{ FromId: userId, ToId: msgBody.Id, Msg: msgBody.Msg}
		msgStr, err := json.Marshal(chMsgBody)
		if err != nil{
			log.Fatalln(err)
			return
		}

		pubErr := rdb.Publish(ctx, userPrefix+ msgBody.Id, msgStr).Err()
		if pubErr != nil {
			panic(any(pubErr))
		}
	}
}

func addUserGroup(userId string, groupId string, webSocket *websocket.Conn)  {
	groupLock.Lock()

	currentGroup, ok := groupUser[groupId]
	if !ok {
		currentGroup = make(map[string]byte)
		groupUser[groupId] = currentGroup
	}
	currentGroup[userId] = 0
	if len(currentGroup)==1 {
		var wg sync.WaitGroup
		wg.Add(1)
		go subGroupMsg(groupPrefix+ groupId, &wg)
		wg.Wait()
	}

	groupLock.Unlock()

	addMsg := fmt.Sprintf("user 【%s】 add  to group 【%s】", userId, groupId)
	if sendErr := websocket.Message.Send(webSocket, addMsg); sendErr != nil {
		log.Printf("group %s user %s can't send\n", groupId, userId)
	}

	chMsgBody:= ChannelMsgBody{ FromId: userId, ToId: groupId, Msg: addMsg}
	msgStr, err := json.Marshal(chMsgBody)
	if err != nil{
		log.Println(err)
		return
	}

	pubErr := rdb.Publish(ctx, groupPrefix+ groupId, msgStr).Err()
	if pubErr != nil {
		log.Println(pubErr)
	}
}

func handleGroup(groupId string, userId string, webSocket *websocket.Conn, msgBody string)  {
	groupLock.RLock()

	users, ok := groupUser[groupId]
	if !ok {
		sendMsg := fmt.Sprintf("group【%s】 not exists", groupId)
		if sendErr := websocket.Message.Send(webSocket, sendMsg); sendErr != nil {
			log.Printf("group %s user %s can't send\n", groupId, userId)
		}
		return
	}

	_, exists := users[userId]
	if !exists {
		sendMsg := fmt.Sprintf("user 【%s】 not in 【%s】", userId, groupId)
		if sendErr := websocket.Message.Send(webSocket, sendMsg); sendErr != nil {
			log.Printf("group %s user %s can't send\n", groupId, userId)
		}
		return
	}

	groupLock.RUnlock()

	log.Printf("group 【%s】 user 【%s】 send:%s",groupId, userId, msgBody)

	chMsgBody:= ChannelMsgBody{ FromId: userId, ToId: groupId, Msg: msgBody}
	msgStr, err := json.Marshal(chMsgBody)
	if err != nil{
		log.Println(err)
		return
	}

	pubErr := rdb.Publish(ctx, groupPrefix+ groupId, msgStr).Err()
	if pubErr != nil {
		log.Println(pubErr)
	}
}

func handleAll(userId string, msgBody interface{})  {
	msg := msgBody.(string)
	log.Printf("user 【%s】 send:%s", userId, msg)

	chMsgBody:= ChannelMsgBody{ FromId: userId,  Msg: msg}
	msgStr, err := json.Marshal(chMsgBody)
	if err != nil{
		log.Println(err)
		return
	}

	pubErr := rdb.Publish(ctx, all, msgStr).Err()
	if pubErr != nil {
		log.Println(pubErr)
	}
}

func subMsg(channel string, wg *sync.WaitGroup){
	sub := rdb.Subscribe(ctx, channel)
	wg.Done()

	subLock.Lock()
	if _, ok := subs[channel]; ok==false {
		subs[channel]=sub
	}
	subLock.Unlock()

	ch := sub.Channel()
	for msg := range ch {
		log.Println(msg.Channel, msg.Payload)

		channelMsgBody := ChannelMsgBody{}
		_ = json.Unmarshal([]byte(msg.Payload), &channelMsgBody)
		if targetWs, ok := users[channelMsgBody.ToId]; ok{
			res := fmt.Sprintf("user %s send:%s", channelMsgBody.FromId, channelMsgBody.Msg)
			if sendErr := websocket.Message.Send(targetWs, res); sendErr != nil {
				log.Printf("%s Can't send\n", channelMsgBody.ToId)
			}
		}
	}
}

func subGroupMsg(channel string, wg *sync.WaitGroup){
	sub := rdb.Subscribe(ctx, channel)
	wg.Done()

	subLock.Lock()
	if _, ok := subs[channel]; ok==false {
		subs[channel]=sub
	}
	subLock.Unlock()

	ch := sub.Channel()
	for msg := range ch {
		log.Println(msg.Channel, msg.Payload)

		channelMsgBody := ChannelMsgBody{}
		_ = json.Unmarshal([]byte(msg.Payload), &channelMsgBody)
		res :=fmt.Sprintf("group 【%s】 user 【%s】 send:%s", channelMsgBody.ToId, channelMsgBody.FromId, channelMsgBody.Msg)
		if currentGroup, ok := groupUser[channelMsgBody.ToId]; ok{
			for user := range currentGroup {
				if user == channelMsgBody.FromId {
					continue
				}
				targetSocket := users[user]
				if sendErr := websocket.Message.Send(targetSocket, res); sendErr != nil {
					log.Printf("%s Can't send\n", user)
				}
			}
		}
	}
}

func subAllMsg(channel string, wg *sync.WaitGroup){
	sub := rdb.Subscribe(ctx, channel)
	wg.Done()

	subLock.Lock()
	if _, ok := subs[channel]; ok==false {
		subs[channel]=sub
	}
	subLock.Unlock()

	ch := sub.Channel()
	for msg := range ch {
		log.Println(msg.Channel, msg.Payload)

		channelMsgBody := ChannelMsgBody{}
		_ = json.Unmarshal([]byte(msg.Payload), &channelMsgBody)
		res :=fmt.Sprintf("user 【%s】 send all:%s", channelMsgBody.FromId, channelMsgBody.Msg)
		for user := range users {
			if user == channelMsgBody.FromId {
				continue
			}

			targetSocket := users[user]
			if sendErr := websocket.Message.Send(targetSocket, res); sendErr != nil {
				log.Printf("%s Can't send\n", user)
			}
		}
	}
}

func init()  {
	users = make(map[string]*websocket.Conn)
	groupUser = make(map[string]map[string]byte)
	subs = make(map[string]*redis.PubSub)
	rdb = redis.NewClient(&redis.Options{ Addr: "localhost:6379" })

	log.SetOutput(os.Stdout)
}
