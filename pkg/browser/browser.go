package browser

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"github.com/dracoDevs/browser-wizard/pkg/page"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

type Event struct {
	Method string                 `json:"method"`
	Params map[string]interface{} `json:"params"`
}

type Browser struct {
	execPath     string
	wsEndpoint   string
	conn         *websocket.Conn
	cmd          *exec.Cmd
	context      context.Context
	cancel       context.CancelFunc
	userDataDir  string
	messageID    int
	messageMutex sync.Mutex
	pid          int
	isHeadless   bool

	eventChan  chan Event
	listenOnce sync.Once
	connMutex  sync.RWMutex

	responseChans map[int]chan map[string]interface{}
	responseMutex sync.Mutex
	readerCancel  context.CancelFunc
}

func GreenLight(execPath string, isHeadless bool, startURL string) *Browser {
	ctx, cancel := context.WithCancel(context.Background())
	userDataDir := filepath.Join(os.TempDir(), fmt.Sprintf("greenlight_%s", uuid.New().String()))

	browser := &Browser{
		execPath:    execPath,
		context:     ctx,
		cancel:      cancel,
		userDataDir: userDataDir,
		isHeadless:  isHeadless,
		eventChan:   make(chan Event, 100),
	}

	if err := browser.launch(startURL); err != nil {
		log.Fatalf("Failed to launch browser: %v", err)
	}

	return browser
}

func (b *Browser) launch(startURL string) error {
	debugPort := "9229"
	args := []string{
		"--remote-debugging-port=" + debugPort,
		"--no-first-run",
		"--user-data-dir=" + b.userDataDir,
		"--remote-allow-origins=*",
		startURL,
	}

	if b.isHeadless {
		args = append(args, "--headless=new")
	}

	b.cmd = exec.CommandContext(b.context, b.execPath, args...)
	if err := b.cmd.Start(); err != nil {
		return fmt.Errorf("failed to start browser: %v", err)
	}

	b.pid = b.cmd.Process.Pid
	log.Printf("Chrome started with PID: %d", b.pid)

	time.Sleep(time.Second)
	if err := b.attachToPage(); err != nil {
		return err
	}

	b.startReader()

	return nil
}

func (b *Browser) attachToPage() error {
	debugPort := "9229"
	resp, err := http.Get(fmt.Sprintf("http://localhost:%s/json", debugPort))
	if err != nil {
		return fmt.Errorf("failed to fetch active pages: %v", err)
	}
	defer resp.Body.Close()

	var pages []map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&pages); err != nil {
		return fmt.Errorf("failed to decode JSON: %v", err)
	}

	for _, page := range pages {
		if page["type"] == "page" && page["url"] != "" {
			if wsURL, ok := page["webSocketDebuggerUrl"].(string); ok {
				// Close existing connection if any
				b.connMutex.Lock()
				oldConn := b.conn
				if oldConn != nil {
					oldConn.Close()
					b.conn = nil
				}
				b.connMutex.Unlock()

				// Add connection timeout
				dialer := websocket.Dialer{
					HandshakeTimeout: 10 * time.Second,
				}

				conn, _, err := dialer.Dial(wsURL, nil)
				if err != nil {
					return fmt.Errorf("failed to connect to page WebSocket: %v", err)
				}

				conn.SetReadLimit(512 * 1024) // 512KB limit
				conn.SetPongHandler(func(string) error {
					conn.SetReadDeadline(time.Now().Add(60 * time.Second))
					return nil
				})

				b.connMutex.Lock()
				b.conn = conn
				b.wsEndpoint = wsURL
				b.connMutex.Unlock()

				log.Printf("Connected to page: %s", page["url"])
				b.startReader() // Only after a new connection is set!
				return nil
			}
		}
	}
	return fmt.Errorf("no suitable page found")
}

func (b *Browser) ResetConnection() error {
	if b.readerCancel != nil {
		b.readerCancel()
	}
	b.connMutex.Lock()
	if b.conn != nil {
		b.conn.Close()
		b.conn = nil
	}
	b.connMutex.Unlock()
	return b.attachToPage()
}

func (b *Browser) getConnection() *websocket.Conn {
	b.connMutex.RLock()
	defer b.connMutex.RUnlock()
	return b.conn
}

func (b *Browser) SendCommandWithResponse(method string, params map[string]interface{}) (map[string]interface{}, error) {
	b.messageMutex.Lock()
	b.messageID++
	id := b.messageID
	b.messageMutex.Unlock()

	message := map[string]interface{}{
		"id":     id,
		"method": method,
		"params": params,
	}

	ch := make(chan map[string]interface{}, 1)
	b.responseMutex.Lock()
	if b.responseChans == nil {
		b.responseChans = make(map[int]chan map[string]interface{})
	}
	b.responseChans[id] = ch
	b.responseMutex.Unlock()

	conn := b.getConnection()
	if conn == nil {
		if err := b.attachToPage(); err != nil {
			return nil, fmt.Errorf("failed to reconnect WebSocket: %v", err)
		}
		conn = b.getConnection()
	}

	if err := conn.WriteJSON(message); err != nil {
		b.responseMutex.Lock()
		delete(b.responseChans, id)
		b.responseMutex.Unlock()
		return nil, fmt.Errorf("failed to send WebSocket message: %v", err)
	}

	select {
	case resp := <-ch:
		b.responseMutex.Lock()
		delete(b.responseChans, id)
		b.responseMutex.Unlock()
		return resp, nil
	case <-time.After(30 * time.Second):
		b.responseMutex.Lock()
		delete(b.responseChans, id)
		b.responseMutex.Unlock()
		return nil, fmt.Errorf("timeout waiting for response to %s", method)
	}
}

func (b *Browser) SendCommandWithoutResponse(method string, params map[string]interface{}) error {
	b.messageMutex.Lock()
	b.messageID++
	id := b.messageID
	b.messageMutex.Unlock()

	message := map[string]interface{}{
		"id":     id,
		"method": method,
		"params": params,
	}

	conn := b.getConnection()
	if conn == nil {
		if err := b.attachToPage(); err != nil {
			return fmt.Errorf("failed to reconnect WebSocket: %v", err)
		}
		conn = b.getConnection()
	}

	if err := conn.WriteJSON(message); err != nil {
		return fmt.Errorf("failed to send WebSocket message: %v", err)
	}

	return nil
}

func (b *Browser) Listen() Event {
	return <-b.eventChan
}

func (b *Browser) startReader() {
	if b.readerCancel != nil {
		b.readerCancel()
	}
	ctx, cancel := context.WithCancel(context.Background())
	b.readerCancel = cancel
	if b.responseChans == nil {
		b.responseChans = make(map[int]chan map[string]interface{})
	}
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			default:
				conn := b.getConnection()
				if conn == nil {
					time.Sleep(100 * time.Millisecond)
					continue
				}
				// conn.SetReadDeadline(time.Now().Add(10 * time.Second))
				_, data, err := conn.ReadMessage()
				if err != nil {
					if nerr, ok := err.(net.Error); ok && nerr.Timeout() {
						continue // just a timeout, keep waiting
					}
					log.Printf("WebSocket reader error: %v", err)
					// Mark the connection as dead so no further reads happen
					b.connMutex.Lock()
					if b.conn == conn {
						b.conn.Close()
						b.conn = nil
					}
					b.connMutex.Unlock()
					return
				}
				var msg map[string]interface{}
				if err := json.Unmarshal(data, &msg); err != nil {
					log.Printf("Failed to parse WebSocket message: %s", string(data))
					continue
				}
				if id, ok := msg["id"].(float64); ok {
					b.responseMutex.Lock()
					ch, exists := b.responseChans[int(id)]
					b.responseMutex.Unlock()
					if exists {
						ch <- msg
					}
				} else if method, ok := msg["method"].(string); ok {
					params, _ := msg["params"].(map[string]interface{})
					select {
					case b.eventChan <- Event{Method: method, Params: params}:
					default:
						// log.Printf("Event channel full, dropping event: %s", method)
					}
				}
			}
		}
	}()
}

func (b *Browser) NewPage() *page.Page {
	if b.getConnection() == nil {
		log.Fatal("WebSocket connection not established. Cannot create a new page.")
	}
	return page.NewPage(b)
}

func (b *Browser) RedLight() {
	b.connMutex.Lock()
	if b.conn != nil {
		if err := b.conn.Close(); err != nil {
			log.Printf("Error closing WebSocket: %v", err)
		}
	}
	b.connMutex.Unlock()

	if b.cmd != nil && b.cmd.Process != nil {
		if err := b.cmd.Process.Kill(); err != nil {
			log.Printf("Error killing browser process: %v", err)
		} else {
			b.cmd.Wait()
		}
	}

	if b.userDataDir != "" {
		if err := os.RemoveAll(b.userDataDir); err != nil {
			log.Printf("Error removing user data directory: %v", err)
		}
	}

	b.cancel()
	log.Println("Browser closed successfully.")
}
