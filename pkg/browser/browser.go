package browser

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
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
	debugPort := "9222"
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

	b.startEventListener()

	return nil
}

func (b *Browser) attachToPage() error {
	debugPort := "9222"
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
				if b.conn != nil {
					b.conn.Close()
				}
				conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
				if err != nil {
					return fmt.Errorf("failed to connect to page WebSocket: %v", err)
				}
				b.conn = conn
				b.wsEndpoint = wsURL
				log.Printf("Connected to page: %s", page["url"])
				b.startEventListener()
				return nil
			}
		}
	}
	return fmt.Errorf("no suitable page found")
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

	if b.conn == nil {
		if err := b.attachToPage(); err != nil {
			return nil, fmt.Errorf("failed to reconnect WebSocket: %v", err)
		}
	}

	if err := b.conn.WriteJSON(message); err != nil {
		return nil, fmt.Errorf("failed to send WebSocket message: %v", err)
	}

	for {
		_, data, err := b.conn.ReadMessage()
		if err != nil {
			return nil, fmt.Errorf("failed to read WebSocket message: %v", err)
		}

		var response map[string]interface{}
		if err := json.Unmarshal(data, &response); err != nil {
			log.Printf("Failed to parse WebSocket message: %s", string(data))
			continue
		}

		// If it's an event, push to eventChan
		if method, ok := response["method"].(string); ok {
			params, _ := response["params"].(map[string]interface{})
			select {
			case b.eventChan <- Event{Method: method, Params: params}:
			default:
			}
			continue
		}

		if responseID, ok := response["id"].(float64); ok && int(responseID) == id {
			return response, nil
		}
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

	if b.conn == nil {
		if err := b.attachToPage(); err != nil {
			return fmt.Errorf("failed to reconnect WebSocket: %v", err)
		}
	}

	if err := b.conn.WriteJSON(message); err != nil {
		return fmt.Errorf("failed to send WebSocket message: %v", err)
	}

	return nil
}

func (b *Browser) Listen() Event {
	b.startEventListener()
	return <-b.eventChan
}

func (b *Browser) startEventListener() {
	b.listenOnce.Do(func() {
		go func() {
			for {
				if b.conn == nil {
					time.Sleep(100 * time.Millisecond)
					continue
				}
				_, data, err := b.conn.ReadMessage()
				if err != nil {
					time.Sleep(100 * time.Millisecond)
					continue
				}
				var msg map[string]interface{}
				if err := json.Unmarshal(data, &msg); err != nil {
					continue
				}
				if method, ok := msg["method"].(string); ok {
					params, _ := msg["params"].(map[string]interface{})
					select {
					case b.eventChan <- Event{Method: method, Params: params}:
					default:
					}
				}
			}
		}()
	})
}

func (b *Browser) NewPage() *page.Page {
	if b.conn == nil {
		log.Fatal("WebSocket connection not established. Cannot create a new page.")
	}
	return page.NewPage(b)
}

func (b *Browser) RedLight() {
	if b.conn != nil {
		if err := b.conn.Close(); err != nil {
			log.Printf("Error closing WebSocket: %v", err)
		}
	}

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
