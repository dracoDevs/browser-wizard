package examples

import (
	"fmt"
	"log"
	"time"

	"github.com/dracoDevs/browser-wizard/pkg/browser"
	"github.com/go-rod/rod/lib/launcher"
)

func TestIframeMonitoring() error {
	chromePath, chromeInstalled := launcher.LookPath()
	if !chromeInstalled {
		return fmt.Errorf("Chrome not found in PATH")
	}

	// Start browser with a site that might have iframes
	b := browser.GreenLight(chromePath, false, "https://x.com", browser.Proxy{})

	// Enable network monitoring
	if err := b.SendCommandWithoutResponse("Network.enable", nil); err != nil {
		return fmt.Errorf("failed to enable network: %v", err)
	}

	page := b.NewPage()
	page.Goto("https://x.com")

	// Monitor for iframes for 10 seconds
	log.Println("Monitoring for iframes...")
	for i := 0; i < 5; i++ {
		time.Sleep(2 * time.Second)

		iframes := b.GetIframeConnections()
		if len(iframes) > 0 {
			log.Printf("Found %d iframe(s):", len(iframes))
			for _, wsURL := range iframes {
				log.Printf("  - %s", wsURL)

				// Try to get the iframe's title
				err := b.SendCommandToIframe(wsURL, "Runtime.evaluate", map[string]interface{}{
					"expression": "document.title",
				})
				if err != nil {
					log.Printf("Failed to send command to iframe: %v", err)
				}
			}
		} else {
			log.Println("No iframes found yet...")
		}
	}

	page.YellowLight(5000)
	b.RedLight()

	return nil
}

func TestIframeWithDynamicContent() error {
	chromePath, chromeInstalled := launcher.LookPath()
	if !chromeInstalled {
		return fmt.Errorf("Chrome not found in PATH")
	}

	// Use a site that dynamically loads iframes
	b := browser.GreenLight(chromePath, false, "https://www.google.com", browser.Proxy{})

	if err := b.SendCommandWithoutResponse("Network.enable", nil); err != nil {
		return fmt.Errorf("failed to enable network: %v", err)
	}

	page := b.NewPage()
	page.Goto("https://www.google.com")

	// Wait a bit for dynamic content to load
	time.Sleep(5 * time.Second)

	// Check for iframes
	iframes := b.GetIframeConnections()
	log.Printf("Found %d iframe(s) after dynamic loading", len(iframes))

	for _, wsURL := range iframes {
		log.Printf("Iframe WebSocket URL: %s", wsURL)

		// Try to interact with the iframe
		err := b.SendCommandToIframe(wsURL, "Runtime.evaluate", map[string]interface{}{
			"expression": "document.readyState",
		})
		if err != nil {
			log.Printf("Failed to interact with iframe: %v", err)
		}
	}

	page.YellowLight(5000)
	b.RedLight()

	return nil
}
