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

type Proxy struct {
	URL      string
	User     string
	Password string
}

type Event struct {
	Method string                 `json:"method"`
	Params map[string]interface{} `json:"params"`
}

type Browser struct {
	execPath            string
	wsEndpoint          string
	conn                *websocket.Conn
	cmd                 *exec.Cmd
	context             context.Context
	cancel              context.CancelFunc
	userDataDir         string
	pid                 int
	isHeadless          bool
	messageID           int
	messageMutex        sync.Mutex
	eventChan           chan Event
	responseChans       map[int]chan map[string]interface{}
	responseMutex       sync.Mutex
	readerCancel        context.CancelFunc
	connMutex           sync.RWMutex
	listenOnce          sync.Once
	Iframes             map[string]*websocket.Conn
	iframeMonitorCancel context.CancelFunc
	proxyUsername       string
	proxyPassword       string
}

func (b *Browser) Listen() Event {
	return <-b.eventChan
}

func (b *Browser) GetIframeConnections() []string {
	b.connMutex.RLock()
	defer b.connMutex.RUnlock()
	var urls []string
	for wsURL := range b.Iframes {
		urls = append(urls, wsURL)
	}
	return urls
}

func (b *Browser) SendCommandToIframe(wsURL, method string, params map[string]interface{}) error {
	b.connMutex.RLock()
	conn, ok := b.Iframes[wsURL]
	b.connMutex.RUnlock()
	if !ok || conn == nil {
		return fmt.Errorf("iframe connection not found: %s", wsURL)
	}
	b.messageMutex.Lock()
	b.messageID++
	id := b.messageID
	b.messageMutex.Unlock()
	msg := map[string]interface{}{"id": id, "method": method, "params": params}
	return conn.WriteJSON(msg)
}

func waitForDevTools(port string, max time.Duration) error {
	deadline := time.Now().Add(max)
	url := "http://127.0.0.1:" + port + "/json"
	for time.Now().Before(deadline) {
		if resp, err := http.Get(url); err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
		}
		time.Sleep(300 * time.Millisecond)
	}
	return fmt.Errorf("DevTools endpoint on %s did not come up within %s", port, max)
}

func (b *Browser) setupProxyAuth() {
	if b.proxyUsername == "" {
		return
	}
	_ = b.SendCommandWithoutResponse("Fetch.enable", map[string]interface{}{
		"handleAuthRequests": true,
	})
	go func() {
		for ev := range b.eventChan {
			reqID, _ := ev.Params["requestId"].(string)
			switch ev.Method {
			case "Fetch.requestPaused":
				_, _ = b.SendCommandWithResponse("Fetch.continueRequest", map[string]interface{}{
					"requestId": reqID,
				})
			case "Fetch.authRequired":
				_ = b.SendCommandWithoutResponse("Fetch.continueWithAuth", map[string]interface{}{
					"requestId": reqID,
					"authChallengeResponse": map[string]interface{}{
						"response": "ProvideCredentials",
						"username": b.proxyUsername,
						"password": b.proxyPassword,
					},
				})
			}
		}
	}()
}

func (b *Browser) launch(startURL string, proxy Proxy) error {
	const debugPort = "9229"
	args := []string{
		"--remote-debugging-port=" + debugPort,
		"--no-first-run",
		"--user-data-dir=" + b.userDataDir,
		"--remote-allow-origins=*",
	}
	if proxy.URL != "" {
		args = append(args, "--proxy-server="+proxy.URL)
	}
	if b.isHeadless {
		args = append(args, "--headless=new")
	}
	args = append(args, "about:blank")
	b.cmd = exec.CommandContext(b.context, b.execPath, args...)
	if err := b.cmd.Start(); err != nil {
		return fmt.Errorf("failed to start browser: %w", err)
	}
	b.pid = b.cmd.Process.Pid
	if err := waitForDevTools(debugPort, 10*time.Second); err != nil {
		return err
	}
	if err := b.attachToPage(); err != nil {
		return err
	}
	b.setupProxyAuth()
	if startURL != "" {
		_ = b.SendCommandWithoutResponse("Network.enable", nil)
		_ = b.SendCommandWithoutResponse("Page.enable", nil)
		_ = b.SendCommandWithoutResponse("Page.navigate", map[string]interface{}{"url": startURL})
	}
	b.startIframeMonitor()
	return nil
}

func (b *Browser) attachToPage() error {
	const debugPort = "9229"
	resp, err := http.Get(fmt.Sprintf("http://localhost:%s/json", debugPort))
	if err != nil {
		return fmt.Errorf("failed to fetch active pages: %v", err)
	}
	defer resp.Body.Close()
	var pages []map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&pages); err != nil {
		return fmt.Errorf("failed to decode JSON: %v", err)
	}
	if b.Iframes == nil {
		b.Iframes = make(map[string]*websocket.Conn)
	}
	for _, p := range pages {
		pageType, _ := p["type"].(string)
		wsURL, _ := p["webSocketDebuggerUrl"].(string)
		pageURL, _ := p["url"].(string)
		if pageType == "iframe" && wsURL != "" {
			b.connMutex.Lock()
			if old, ok := b.Iframes[wsURL]; ok && old != nil {
				old.Close()
			}
			if conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil); err == nil {
				conn.SetReadLimit(512 * 1024)
				b.Iframes[wsURL] = conn
				log.Printf("Connected to iframe: %s", pageURL)
			}
			b.connMutex.Unlock()
			continue
		}
		if pageType == "page" && wsURL != "" {
			conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
			if err != nil {
				return fmt.Errorf("failed to connect to page WebSocket: %v", err)
			}
			conn.SetReadLimit(512 * 1024)
			b.connMutex.Lock()
			if b.conn != nil {
				b.conn.Close()
			}
			b.conn = conn
			b.wsEndpoint = wsURL
			b.connMutex.Unlock()
			log.Printf("Connected to page: %s", pageURL)
			b.startReader()
			return nil
		}
	}
	return fmt.Errorf("no suitable page found")
}

func GreenLight(execPath string, isHeadless bool, startURL string, proxy Proxy) *Browser {
	ctx, cancel := context.WithCancel(context.Background())
	userDataDir := filepath.Join(os.TempDir(), fmt.Sprintf("greenlight_%s", uuid.New().String()))
	b := &Browser{
		execPath:      execPath,
		context:       ctx,
		cancel:        cancel,
		userDataDir:   userDataDir,
		isHeadless:    isHeadless,
		proxyUsername: proxy.User,
		proxyPassword: proxy.Password,
		eventChan:     make(chan Event, 100),
		responseChans: make(map[int]chan map[string]interface{}),
	}
	if err := b.launch(startURL, proxy); err != nil {
		log.Fatalf("Failed to launch browser: %v", err)
	}
	return b
}

func (b *Browser) SendCommandWithoutResponse(method string, params map[string]interface{}) error {
	b.messageMutex.Lock()
	b.messageID++
	id := b.messageID
	b.messageMutex.Unlock()
	msg := map[string]interface{}{"id": id, "method": method, "params": params}
	conn := b.getConnection()
	if conn == nil {
		if err := b.attachToPage(); err != nil {
			return fmt.Errorf("reconnect: %v", err)
		}
		conn = b.getConnection()
	}
	return conn.WriteJSON(msg)
}

func (b *Browser) SendCommandWithResponse(method string, params map[string]interface{}) (map[string]interface{}, error) {
	b.messageMutex.Lock()
	b.messageID++
	id := b.messageID
	b.messageMutex.Unlock()
	msg := map[string]interface{}{"id": id, "method": method, "params": params}
	ch := make(chan map[string]interface{}, 1)
	b.responseMutex.Lock()
	b.responseChans[id] = ch
	b.responseMutex.Unlock()
	conn := b.getConnection()
	if conn == nil {
		if err := b.attachToPage(); err != nil {
			return nil, fmt.Errorf("reconnect: %v", err)
		}
		conn = b.getConnection()
	}
	if err := conn.WriteJSON(msg); err != nil {
		return nil, err
	}
	select {
	case resp := <-ch:
		return resp, nil
	case <-time.After(30 * time.Second):
		return nil, fmt.Errorf("timeout waiting for %s", method)
	}
}

func (b *Browser) getConnection() *websocket.Conn {
	b.connMutex.RLock()
	defer b.connMutex.RUnlock()
	return b.conn
}

func (b *Browser) startReader() {
	if b.readerCancel != nil {
		b.readerCancel()
	}
	ctx, cancel := context.WithCancel(context.Background())
	b.readerCancel = cancel
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
				_, data, err := conn.ReadMessage()
				if err != nil {
					b.connMutex.Lock()
					if b.conn == conn {
						conn.Close()
						b.conn = nil
					}
					b.connMutex.Unlock()
					return
				}
				var msg map[string]interface{}
				if err := json.Unmarshal(data, &msg); err != nil {
					continue
				}
				if id, ok := msg["id"].(float64); ok {
					b.responseMutex.Lock()
					if ch, exists := b.responseChans[int(id)]; exists {
						ch <- msg
					}
					b.responseMutex.Unlock()
				} else if m, ok := msg["method"].(string); ok {
					params, _ := msg["params"].(map[string]interface{})
					select {
					case b.eventChan <- Event{Method: m, Params: params}:
					default:
					}
				}
			}
		}
	}()
}

func (b *Browser) startIframeMonitor() {
	if b.iframeMonitorCancel != nil {
		b.iframeMonitorCancel()
	}
	ctx, cancel := context.WithCancel(context.Background())
	b.iframeMonitorCancel = cancel
	ticker := time.NewTicker(2 * time.Second)
	go func() {
		for {
			select {
			case <-ctx.Done():
				ticker.Stop()
				return
			case <-ticker.C:
				b.checkForNewIframes()
			}
		}
	}()
}

func (b *Browser) checkForNewIframes() {
	const debugPort = "9229"
	resp, err := http.Get(fmt.Sprintf("http://localhost:%s/json", debugPort))
	if err != nil {
		return
	}
	defer resp.Body.Close()
	var pages []map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&pages); err != nil {
		return
	}
	b.connMutex.Lock()
	defer b.connMutex.Unlock()
	for _, p := range pages {
		if p["type"] != "iframe" {
			continue
		}
		wsURL, _ := p["webSocketDebuggerUrl"].(string)
		urlStr, _ := p["url"].(string)
		if _, exists := b.Iframes[urlStr]; exists || wsURL == "" {
			continue
		}
		if conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil); err == nil {
			conn.SetReadLimit(512 * 1024)
			b.Iframes[urlStr] = conn
		}
	}
	for urlStr, conn := range b.Iframes {
		if conn == nil {
			delete(b.Iframes, urlStr)
			continue
		}
		if err := conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(time.Second)); err != nil {
			conn.Close()
			delete(b.Iframes, urlStr)
		}
	}
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

func (b *Browser) RedLight() {
	if b.iframeMonitorCancel != nil {
		b.iframeMonitorCancel()
	}
	b.connMutex.Lock()
	for _, c := range b.Iframes {
		if c != nil {
			c.Close()
		}
	}
	b.Iframes = make(map[string]*websocket.Conn)
	if b.conn != nil {
		b.conn.Close()
	}
	b.connMutex.Unlock()
	if b.cmd != nil && b.cmd.Process != nil {
		_ = b.cmd.Process.Kill()
		b.cmd.Wait()
	}
	if b.userDataDir != "" {
		_ = os.RemoveAll(b.userDataDir)
	}
	b.cancel()
}

func (b *Browser) NewPage() *page.Page {
	if b.getConnection() == nil {
		log.Fatal("WebSocket connection not established")
	}
	return page.NewPage(b)
}
