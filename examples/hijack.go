package examples

import (
	"fmt"
	"log"

	"github.com/dracoDevs/browser-wizard/pkg/browser"
	"github.com/go-rod/rod/lib/launcher"
)

func TestHijackRequests() error {
	chromePath, chromeInstalled := launcher.LookPath()
	if !chromeInstalled {
		return fmt.Errorf("Chrome not found in PATH")
	}
	b := browser.GreenLight(chromePath, false, "https://example.com")

	if err := b.SendCommandWithoutResponse("Network.enable", nil); err != nil {
		return fmt.Errorf("failed to enable network: %v", err)
	}

	b.SendCommandWithoutResponse("Network.setRequestInterception", map[string]interface{}{
		"patterns": []map[string]interface{}{{"urlPattern": "*"}},
	})

	go func() {
		for {
			event := b.Listen()
			if event.Method == "Network.requestWillBeSent" {
				if reqID, ok := event.Params["requestId"].(string); ok {
					if req, ok := event.Params["request"].(map[string]interface{}); ok {
						if url, ok := req["url"].(string); ok {
							log.Printf("Intercepted request: %s | Request ID: %s", url, reqID)
						}
					}
				}
			}
		}
	}()

	page := b.NewPage()
	page.Goto("https://example.com")
	page.YellowLight(5000)

	b.RedLight()

	return nil
}

func TestHijackModifyRequest() error {
	chromePath, chromeInstalled := launcher.LookPath()
	if !chromeInstalled {
		return fmt.Errorf("Chrome not found in PATH")
	}
	b := browser.GreenLight(chromePath, false, "https://example.com")

	if err := b.SendCommandWithoutResponse("Network.enable", nil); err != nil {
		return fmt.Errorf("failed to enable network: %v", err)
	}

	b.SendCommandWithoutResponse("Network.setRequestInterception", map[string]interface{}{
		"patterns": []map[string]interface{}{{"urlPattern": "*"}},
	})

	go func() {
		for {
			event := b.Listen()
			if event.Method == "Network.requestIntercepted" {
				reqID, ok := event.Params["interceptionId"].(string)
				if !ok {
					continue
				}
				headers := map[string]interface{}{
					"X-My-Header": "ModifiedByHijack",
				}
				err := b.SendCommandWithoutResponse("Network.continueInterceptedRequest", map[string]interface{}{
					"interceptionId": reqID,
					"headers":        headers,
				})
				if err != nil {
					log.Printf("Failed to continue intercepted request: %v", err)
				} else {
					log.Printf("Modified and continued request: %s", reqID)
				}
			}
		}
	}()

	page := b.NewPage()
	page.Goto("https://example.com")
	page.YellowLight(5000)

	b.RedLight()

	return nil
}
