package examples

import (
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/dracoDevs/browser-wizard/pkg/browser"
	"github.com/go-rod/rod/lib/launcher"
)

func proxyPieces(raw string) (urlOnly, user, pass string) {
	p := strings.Split(raw, ":")
	if len(p) == 4 {
		urlOnly = p[0] + ":" + p[1] // host:port  (no creds)
		user, pass = p[2], p[3]
	} else { // ip:port
		urlOnly = raw
	}
	return
}

func TestProxyUsage() {
	raw := "YOUR_PROXY_HERE"
	proxyURL, user, password := proxyPieces(raw)

	fmt.Println("Using proxy:", proxyURL)

	chromePath, ok := launcher.LookPath()
	if !ok {
		log.Fatal("Chrome not found")
	}

	b := browser.GreenLight(chromePath, false, "https://www.whatismyip.com/", browser.Proxy{
		URL:      proxyURL,
		User:     user,
		Password: password,
	})
	defer b.RedLight()

	time.Sleep(30 * time.Second)
}
