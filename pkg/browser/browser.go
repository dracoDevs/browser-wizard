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

	"github.com/bosniankicks/greenlight/pkg/page"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/mafredri/cdp"
)

type Browser struct {
	execPath     string
	wsEndpoint   string
	wsConn       *websocket.Conn // renamed to avoid conflict
	cmd          *exec.Cmd
	context      context.Context
	cancel       context.CancelFunc
	userDataDir  string
	messageID    int
	messageMutex sync.Mutex
	pid          int
	isHeadless   bool

	conn   *cdp.Conn       // this is the CDP connection
	events chan *cdp.Event // CDP event channel
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

	b.cmd = exec.CommandContext(b.wsConnext, b.execPath, args...)
	if err := b.cmd.Start(); err != nil {
		return fmt.Errorf("failed to start browser: %v", err)
	}

	b.pid = b.cmd.Process.Pid
	log.Printf("Chrome started with PID: %d", b.pid)

	time.Sleep(time.Second)
	if err := b.attachToPage(); err != nil {
		return err
	}

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
				if b.wsConn != nil {
					b.wsConn.Close()
				}
				conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
				if err != nil {
					return fmt.Errorf("failed to connect to page WebSocket: %v", err)
				}
				b.wsConn = conn
				b.wsEndpoint = wsURL
				log.Printf("Connected to page: %s", page["url"])
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

	if b.wsConn == nil {
		if err := b.attachToPage(); err != nil {
			return nil, fmt.Errorf("failed to reconnect WebSocket: %v", err)
		}
	}

	if err := b.wsConn.WriteJSON(message); err != nil {
		return nil, fmt.Errorf("failed to send WebSocket message: %v", err)
	}

	for {
		_, data, err := b.wsConn.ReadMessage()
		if err != nil {
			return nil, fmt.Errorf("failed to read WebSocket message: %v", err)
		}

		var response map[string]interface{}
		if err := json.Unmarshal(data, &response); err != nil {
			log.Printf("Failed to parse WebSocket message: %s", string(data))
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

	if b.wsConn == nil {
		if err := b.attachToPage(); err != nil {
			return fmt.Errorf("failed to reconnect WebSocket: %v", err)
		}
	}

	if err := b.wsConn.WriteJSON(message); err != nil {
		return fmt.Errorf("failed to send WebSocket message: %v", err)
	}

	return nil
}

func (b *Browser) NewPage() *page.Page {
	if b.wsConn == nil {
		log.Fatal("WebSocket connection not established. Cannot create a new page.")
	}
	return page.NewPage(b)
}

func (b *Browser) RedLight() {
	if b.wsConn != nil {
		if err := b.wsConn.Close(); err != nil {
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
