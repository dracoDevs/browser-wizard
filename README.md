<p align="center">
  <img src="./logo.png" height="300" width="350" alt="Browser Wizard Logo" />
</p>

# 🔗Browser Wizard

⚠️ This project is currently unfit for production at this time so implement at your own risk.

# 🔗Purpose

This purpose of this project is to intuitively change the way us webscrapers scrape websites with various Anti-Bot protections.

# Usage

Browser Wizard provides powerful capabilities for intercepting and modifying network requests, as well as ensuring network stability during web scraping operations.

## Request Hijacking

The `/examples/hijack.go` file demonstrates how to intercept and modify network requests:

### Basic Request Interception

```go
// Enable network interception
b.SendCommandWithoutResponse("Network.setRequestInterception", map[string]interface{}{
    "patterns": []map[string]interface{}{{"urlPattern": "*"}},
})

// Listen for intercepted requests
go func() {
    for {
        event := b.Listen()
        if event.Method == "Network.requestWillBeSent" {
            // Handle intercepted request
        }
    }
}()
```

### Modifying Requests

You can modify request headers and continue the intercepted request:

```go
if event.Method == "Network.requestIntercepted" {
    reqID := event.Params["interceptionId"].(string)
    headers := map[string]interface{}{
        "X-My-Header": "ModifiedByHijack",
    }
    b.SendCommandWithoutResponse("Network.continueInterceptedRequest", map[string]interface{}{
        "interceptionId": reqID,
        "headers":        headers,
    })
}
```

## Network Stability

The `/examples/stability.go` file shows how to wait for network stability before proceeding:

### Wait for Network Stability

```go
func WaitForNetworkStability(b *browser.Browser) error {
    // Monitor network activity and wait for stability
    // Returns when network activity has been stable for 1.5 seconds
}
```

This is particularly useful for sites with dynamic content loading or anti-bot protections that make multiple requests.

## Basic Setup

```go
chromePath, chromeInstalled := launcher.LookPath()
if !chromeInstalled {
    return fmt.Errorf("Chrome not found in PATH")
}

// Initialize browser
b := browser.GreenLight(chromePath, false, "https://example.com")

// Enable network monitoring
b.SendCommandWithoutResponse("Network.enable", nil)

// Create page and navigate
page := b.NewPage()
page.Goto("https://example.com")

// Wait for stability or add delays as needed
page.YellowLight(5000)

// Clean up
b.RedLight()
```

## Iframe Monitoring

Browser Wizard automatically monitors for new iframes every 2 seconds and maintains WebSocket connections to them:

```go
// Get list of active iframe connections
iframes := b.GetIframeConnections()
for _, wsURL := range iframes {
    log.Printf("Active iframe: %s", wsURL)
}

// Send commands to specific iframes
err := b.SendCommandToIframe(wsURL, "Runtime.evaluate", map[string]interface{}{
    "expression": "document.title",
})
```

This is particularly useful for:

-   **Dynamic content**: Sites that load iframes via JavaScript
-   **Anti-bot detection**: Some sites use iframes to load protected content
-   **Cross-origin scraping**: Accessing content from different domains
-   **SPA monitoring**: Single Page Applications that inject iframes dynamically

## Performance Optimizations

Browser Wizard includes several optimizations to handle high-traffic websites:

-   **Increased WebSocket read limit**: 10MB (up from 512KB) to handle heavy network traffic
-   **Larger event buffer**: 1000 events (up from 100) to prevent event dropping
-   **Automatic reconnection**: Handles "read limit exceeded" errors gracefully
-   **Selective event filtering**: Only process events you actually need
-   **Network buffer optimization**: Configurable buffer sizes for different network scenarios
-   **Automatic iframe monitoring**: Continuously checks for new iframes every 2 seconds

# THIS IS A FORK

All credit for the foundation of this project to https://github.com/bosniankicks/greenlight big thanks!
