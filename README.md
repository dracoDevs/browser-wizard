# GREENLIGHT 🚦

Greenlight is the first undetected Golang based Web-Automation Framework. Greenlight was made to create a way for Golang devs to utilize the lightweight code for fast automation browsing. With added error handling internally, the user can write their code just like playwright in python! No more err pain! Greenlight was named after the traffic light system. Greenlight, or go starts the browser initialization. YellowLight slows down the browser or does a wait/sleep function. Finally RedLight closes the browser and comes to a complete stop. Usage shown below :) 

## Installation

```go
go get github.com/bosniankicks/greenlight
go mod tidy
```

## Components

```go
GreenLight(chromePath string, headless bool, startURL string)

Initializes Chrome browser instance
Args:

- chromePath: Path to Chrome executable
- headless: Run browser without GUI
- startURL: Starting URL

- Returns: Browser instance
```

```go
RedLight()

- Closes browser and cleans up resources
```

```go
page.Goto(url string)

- Navigates to specified URL
```

```go
page.YellowLight(milliseconds int)

- Pauses execution for specified milliseconds (same usage as waitForTimeout)
```

```go
page.Locator(selector string)

- Finds element using CSS selector
- Returns: Locator object
```

```go
locator.Fill(value string)

- Clears and fills input element
- Built-in 30s timeout, retries every 350ms
```

```go
locator.Click()

- Clicks element
- Built-in 30s timeout
```

## Typing Functions (Added)


```go
locator.TypeSequentially(text string, delayMs int)

- Types text with delay between each character

Args:
text: String to type
delayMs: Millisecond delay between keystrokes
```

```go
locator.TypeWithMistakes(text string, delayMs int)

- Types with human-like mistakes and corrections
- Randomly adds typos and backspaces

Args:
text: String to type
delayMs: Base typing speed
```

## Usage

```go
package main

import (
    browser "github.com/bosniankicks/greenlight/pkg/browser"
)

func main() {
    // Initialize browser - use your browser path wanted 
    // keep headless as false for now (not enough testing)
    // if you want to force the browser to begin the automation at a URL right away, 
    // it will auto open the url inside the greenlight func.
    // If you want a slower load visit google first or example.com then use goto. 

    b := browser.GreenLight("/Applications/Google Chrome.app/Contents/MacOS/Google Chrome", false, "https://example.com")
    defer b.RedLight() //either use this to auto close the browser or use b.RedLight() at the end of the script

    // Get page object
    page := b.NewPage()
    
    // Navigate to URL wanted (use this as a reload or refresh of the same site if needed)
    page.Goto("https://login.example.com")
    
    // Fill form fields -- fills off locator input then can fill (paste) or type in different ways
    page.Locator("#email").Fill("user@email.com")
    page.Locator("#password").TypeWithMistakes("password123", 100)
    
    // Click submit
    page.Locator("#submit").Click()
    
    // Wait 3 seconds -- 3 second delay or timeout
    page.YellowLight(3000)
}
```

## Contributing

Pull requests are welcome. Dm me on discord @pickumaternu if you have an idea thats realistic in automation. No I will not tell you how to make an akamai gen or recap gen or anything of a gen. Left out cookie saving this time on purpose. 

The framework lacks functionality as you only really need to input things and click buttons in a framework. I didnt see the need to add some features like routes or whatever as this should be a basic framework against detection. 

This does not work on CLOUDFLARE or any CAPTCHA, helps users bypass fingerprint anitbots such as Akamai, Kasada, DataDome, and others. 

Checkout my previous work @ https://github.com/bosniankicks/Kurva-Krome

Send me money as support for redbulls and geek bars -- cashapp -- $bosniankicks

## License

[MIT](https://choosealicense.com/licenses/mit/)

# Capsolver Sponsor
[![CapSolver Ads](https://github.com/user-attachments/assets/793acd61-2ad9-46cf-bdec-60a61be962e1)](https://www.capsolver.com/?utm_source=github&utm_medium=repo&utm_campaign=scraping&utm_term=Kurva-Krome)

Alternatively, you may use Capsolver. For more information and to get started, visit the official [Capsolver website](https://www.capsolver.com/?utm_source=github&utm_medium=repo&utm_campaign=scraping&utm_term=Kurva-Krome).
