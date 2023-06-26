package main

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/go-redis/redis/v8"
	"log"
	"net/http"
	"sync"
	"time"
)

// SSEServer SSE服务器
type SSEServer struct {
	connections map[string][]chan string
	mutex       sync.Mutex
	redisClient *redis.Client
}

// _sseServer sse服务指针
var _sseServer *SSEServer

// Message 消息
type Message struct {
	UserId string      `json:"user_id"`
	Event  string      `json:"event"`
	Data   interface{} `json:"data"`
}

func main() {
	initSSE()
	// 注册 SSE 事件流处理器 http服务
	http.HandleFunc("/", httpServer)
	// 启动订阅 Redis 频道并处理消息
	go _sseServer.subscribeRedisChannel()
	// 定时推送空数据到客户端
	go _sseServer.timedPush()
	// 启动服务器
	log.Println("SSE server is running on :8085")
	log.Fatal(http.ListenAndServe(":8085", nil))
}

// initSSE 实例化结构体
func initSSE() {
	_sseServer = &SSEServer{
		connections: make(map[string][]chan string),
		redisClient: redis.NewClient(&redis.Options{
			Addr:     "localhost:6379",
			Password: "", // 密码
			DB:       1,  // 选择相应的数据库
		}),
	}
}

// httpServer 注册 SSE 事件流处理器 http服务
func httpServer(w http.ResponseWriter, r *http.Request) {
	// 创建 SSE 事件流处理器
	// 获取用户ID，假设从查询参数中获取，你可以根据实际情况修改
	userID := r.URL.Query().Get("user")

	// 设置响应头，表明服务器支持 SSE
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	// 创建 SSE 连接通道
	messageChan := make(chan string)

	// 注册 SSE 连接通道
	_sseServer.registerConnection(userID, messageChan)
	defer _sseServer.unregisterConnection(userID)

	// 推送消息给客户端
	for msg := range messageChan {
		// 将消息发送给客户端
		fmt.Fprintf(w, "data: %s\n\n", msg)
		// 刷新 SSE 连接，确保消息立即发送给客户端
		w.(http.Flusher).Flush()
	}

}

// 注册 SSE 连接
func (s *SSEServer) registerConnection(userID string, messageChan chan string) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	//追加链接进入
	s.connections[userID] = append(s.connections[userID], messageChan)
}

// 注销 SSE 连接
func (s *SSEServer) unregisterConnection(userID string) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	delete(s.connections, userID)
}

// 发送消息给指定用户
func (s *SSEServer) sendMessageToUser(userID, message string) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if userMap, ok := s.connections[userID]; ok {

		for _, conn := range userMap {
			conn <- message
		}
		//打印发出的消息
		fmt.Println(message)
	}
}

// 订阅 Redis 频道并处理消息
func (s *SSEServer) subscribeRedisChannel() {
	pubsub := s.redisClient.Subscribe(context.Background(), "hdpro_database_web_message")
	defer pubsub.Close()

	ch := pubsub.Channel()

	for msg := range ch {
		// 从 Redis 频道接收到消息
		_message := Message{}
		json.Unmarshal([]byte(msg.Payload), &_message)
		// 发送消息给指定用户
		s.sendMessageToUser(_message.UserId, msg.Payload)
	}
}

// 定时给客户端推送空消息，保持连接
func (s *SSEServer) timedPush() {
	// 我们设置的服务器响应超时时间为5分钟，保险起见，每3分钟给客户端推一次空消息用来保持连接
	for key := range s.connections {
		// 发送消息给指定用户
		jsonByte, _ := json.Marshal(Message{
			key, "test", nil,
		})
		s.sendMessageToUser(key, string(jsonByte))
	}
	time.Sleep(10 * time.Second)

	s.timedPush()
}
