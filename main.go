package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"

	"github.com/go-redis/redis/v8"
)

// SSEServer 表示SSE服务器
type SSEServer struct {
	connections map[string]chan string
	mutex       sync.Mutex
	redisClient *redis.Client
}

// Message 消息
type Message struct {
	UserId string      `json:"user_id"`
	Event  string      `json:"event"`
	Data   interface{} `json:"data"`
}

func main() {
	sseServer := &SSEServer{
		connections: make(map[string]chan string),
		redisClient: redis.NewClient(&redis.Options{
			Addr:     "localhost:6379",
			Password: "", // 密码
			DB:       1,  // 选择相应的数据库
		}),
	}

	// 创建 SSE 事件流处理器
	sseHandler := func(w http.ResponseWriter, r *http.Request) {
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
		sseServer.registerConnection(userID, messageChan)
		defer sseServer.unregisterConnection(userID)

		// 推送消息给客户端
		for msg := range messageChan {
			// 将消息发送给客户端
			fmt.Fprintf(w, "data: %s\n\n", msg)
			// 刷新 SSE 连接，确保消息立即发送给客户端
			w.(http.Flusher).Flush()
		}
	}

	// 注册 SSE 事件流处理器
	http.HandleFunc("/events", sseHandler)

	// 启动订阅 Redis 频道并处理消息
	go sseServer.subscribeRedisChannel()

	// 启动服务器
	log.Println("SSE server is running on :8085")
	log.Fatal(http.ListenAndServe(":8085", nil))
}

// 注册 SSE 连接
func (s *SSEServer) registerConnection(userID string, messageChan chan string) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	s.connections[userID] = messageChan
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

	if conn, ok := s.connections[userID]; ok {
		conn <- message
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

		fmt.Println(_message)
		// 发送消息给指定用户
		s.sendMessageToUser(_message.UserId, _message.Event)
	}
}
