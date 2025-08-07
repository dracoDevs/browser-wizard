package examples

import (
	"fmt"
	"log"
	"time"

	"github.com/dracoDevs/browser-wizard/pkg/browser"
	"github.com/go-rod/rod/lib/launcher"
)

func WaitForNetworkStability(b *browser.Browser) error {
	for {
		if browser.ProxyHandled {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	_ = b.SendCommandWithoutResponse("Fetch.enable", nil)

	const stableDuration = time.Second + 500*time.Millisecond
	const pollInterval = 300 * time.Millisecond

	lastRequestPaused := time.Now()
	done := make(chan struct{})

	go func() {
		for ev := range b.EventChan {
			switch ev.Method {
			case "Fetch.requestPaused":
				lastRequestPaused = time.Now()
				reqID, _ := ev.Params["requestId"].(string)
				_, _ = b.SendCommandWithResponse("Fetch.continueRequest", map[string]interface{}{
					"requestId": reqID,
				})
			}
		}
	}()

	for {
		if time.Since(lastRequestPaused) >= stableDuration {
			break
		}
		time.Sleep(pollInterval)
	}

	close(done)
	return nil
}

func TestWaitForNetworkStability() error {
	chromePath, chromeInstalled := launcher.LookPath()
	if !chromeInstalled {
		return fmt.Errorf("Chrome not found in PATH")
	}

	b := browser.GreenLight(chromePath, false, "https://x.com", browser.Proxy{})

	page := b.NewPage()
	page.Goto("https://x.com")

	if err := WaitForNetworkStability(b); err != nil {
		return fmt.Errorf("network stability check failed: %v", err)
	}

	log.Println("Network is stable, proceeding with tests...")

	page.YellowLight(50000)

	b.RedLight()

	return nil
}
