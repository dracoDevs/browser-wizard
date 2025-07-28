package examples

import (
	"fmt"
	"log"
	"time"

	"github.com/dracoDevs/browser-wizard/pkg/browser"
	"github.com/go-rod/rod/lib/launcher"
)

func WaitForNetworkStability(b *browser.Browser) error {
	var lastVal interface{}
	var stableSince time.Time
	const stableDuration = time.Second + 500*time.Millisecond

	for {
		res, err := b.SendCommandWithResponse("Runtime.evaluate", map[string]interface{}{
			"expression": `(function() {
                const entries = performance.getEntriesByType("resource");
                return entries.filter(e => e.initiatorType === "xmlhttprequest" || e.initiatorType === "fetch").length;
            })()`,
			"returnByValue": true,
		})
		if err != nil {
			return fmt.Errorf("error evaluating script: %v", err)
		}

		parentVal := res["result"].(map[string]interface{})
		subparentVal := parentVal["result"].(map[string]interface{})
		val := subparentVal["value"]

		if lastVal == nil || val != lastVal {
			lastVal = val
			stableSince = time.Now()
		} else if time.Since(stableSince) >= stableDuration {
			break
		}
		time.Sleep(300 * time.Millisecond)
	}
	return nil
}

func TestWaitForNetworkStability() error {
	chromePath, chromeInstalled := launcher.LookPath()
	if !chromeInstalled {
		return fmt.Errorf("Chrome not found in PATH")
	}

	b := browser.GreenLight(chromePath, false, "https://x.com")

	if err := b.SendCommandWithoutResponse("Network.enable", nil); err != nil {
		return fmt.Errorf("failed to enable network: %v", err)
	}

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
