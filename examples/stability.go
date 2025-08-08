package examples

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/dracoDevs/browser-wizard/pkg/browser"
	"github.com/go-rod/rod/lib/launcher"
)

func getTopFrameURL(b *browser.Browser) (string, error) {
	resp, err := b.SendCommandWithResponse("Page.getFrameTree", nil)
	if err != nil {
		return "", fmt.Errorf("Page.getFrameTree failed: %w", err)
	}
	ft, ok := resp["frameTree"].(map[string]interface{})
	if !ok {
		return "", errors.New("missing frameTree")
	}
	frame, ok := ft["frame"].(map[string]interface{})
	if !ok {
		return "", errors.New("missing frame")
	}
	url, _ := frame["url"].(string)
	return url, nil
}

func WaitForNetworkStability(ctx context.Context, b *browser.Browser, stableFor time.Duration) error {
	if u, err := getTopFrameURL(b); err == nil {
		log.Println("Current page URL:", u)
	}
	proxyTicker := time.NewTicker(100 * time.Millisecond)
	defer proxyTicker.Stop()
	for {
		if browser.ProxyHandled {
			break
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("proxy not ready: %w", ctx.Err())
		case <-proxyTicker.C:
		}
	}
	if err := b.SendCommandWithoutResponse("Fetch.enable", map[string]interface{}{
		"patterns": []map[string]interface{}{
			{"urlPattern": "*"},
		},
	}); err != nil {
		return fmt.Errorf("failed to enable Fetch: %w", err)
	}
	defer func() { _ = b.SendCommandWithoutResponse("Fetch.disable", nil) }()
	lastActivity := time.Now()
	watchCtx, stopWatch := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			select {
			case <-watchCtx.Done():
				return
			case ev, ok := <-b.EventChan:
				if !ok {
					return
				}
				if ev.Method == "Fetch.requestPaused" {
					lastActivity = time.Now()
					if reqID, _ := ev.Params["requestId"].(string); reqID != "" {
						_, _ = b.SendCommandWithResponse("Fetch.continueRequest", map[string]interface{}{"requestId": reqID})
					}
				}
			}
		}
	}()
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			stopWatch()
			<-done
			return ctx.Err()
		case <-ticker.C:
			if time.Since(lastActivity) >= stableFor {
				stopWatch()
				<-done
				return nil
			}
		}
	}
}

func TestWaitForNetworkStability() error {
	chromePath, chromeInstalled := launcher.LookPath()
	if !chromeInstalled {
		return fmt.Errorf("Chrome not found in PATH")
	}
	b := browser.GreenLight(chromePath, false, "https://x.com", browser.Proxy{})
	page := b.NewPage()
	page.Goto("https://x.com")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := WaitForNetworkStability(ctx, b, 2500*time.Millisecond); err != nil {
		return fmt.Errorf("network stability check failed: %v", err)
	}
	log.Println("Network is stable, proceeding with tests...")
	page.YellowLight(50000)
	b.RedLight()
	return nil
}
