package main

import (
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/mmcdole/gofeed"
	"golang.org/x/sync/semaphore"
)

const (
	concurrencyLimit = 60
	timeoutSeconds   = 30
	maxRetries       = 3
)

type ValidationResult struct {
	URL        string
	Status     string
	Message    string
	ItemCount  int
	LastUpdate time.Time
}

func validateFeed(url string, client *http.Client, parser *gofeed.Parser) ValidationResult {
	url = strings.TrimSpace(url)

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeoutSeconds)*time.Second)
	defer cancel()

	req, reqErr := http.NewRequestWithContext(ctx, "GET", url, nil)
	if reqErr != nil {
		return ValidationResult{URL: url, Status: "invalid", Message: "Invalid URL: " + reqErr.Error()}
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; FeedValidator/1.0)")
	req.Header.Set("Accept-Language", "en-US;q=0.7,en;q=0.3")

	var resp *http.Response
	var err error
	var backoff time.Duration = 1

	for attempt := 1; attempt <= maxRetries; attempt++ {
		resp, err = client.Do(req)

		if err != nil {
			// Check specifically for context canceled errors
			if strings.Contains(err.Error(), "context canceled") || strings.Contains(err.Error(), "context deadline exceeded") {
				fmt.Fprintf(os.Stderr, "Timeout on attempt %d/%d for %s: %v\n", attempt, maxRetries, url, err)
			} else {
				fmt.Fprintf(os.Stderr, "Error on attempt %d/%d for %s: %v\n", attempt, maxRetries, url, err)
			}

			if attempt == maxRetries {
				break
			}

			time.Sleep(backoff * time.Second)
			backoff *= 2 // Exponential backoff
			continue
		}

		if resp.StatusCode != 200 {
			errMsg := fmt.Sprintf("HTTP status %d", resp.StatusCode)
			resp.Body.Close()

			// Don't retry client errors (4xx) except 429 (too many requests)
			if resp.StatusCode >= 400 && resp.StatusCode < 500 && resp.StatusCode != 429 {
				return ValidationResult{URL: url, Status: "invalid", Message: errMsg}
			}

			fmt.Fprintf(os.Stderr, "Retry %d/%d for %s: %v\n", attempt, maxRetries, url, errMsg)

			if attempt == maxRetries {
				break
			}

			time.Sleep(backoff * time.Second)
			backoff *= 2
			continue
		}

		// If we got here, we have a successful response
		break
	}

	if err != nil {
		// Check specifically for timeout errors
		if strings.Contains(err.Error(), "context canceled") || strings.Contains(err.Error(), "context deadline exceeded") {
			return ValidationResult{URL: url, Status: "transient", Message: "Request timed out after " + fmt.Sprintf("%d", timeoutSeconds) + " seconds"}
		}
		return ValidationResult{URL: url, Status: "transient", Message: err.Error()}
	}

	if resp == nil || resp.StatusCode != 200 {
		statusCode := 0
		if resp != nil {
			statusCode = resp.StatusCode
		}
		return ValidationResult{URL: url, Status: "transient", Message: fmt.Sprintf("Failed after %d attempts, last status: %d", maxRetries, statusCode)}
	}

	defer resp.Body.Close()

	// Read the entire body to avoid "unexpected EOF" errors
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return ValidationResult{URL: url, Status: "transient", Message: "Error reading response: " + err.Error()}
	}

	bodyReader := strings.NewReader(string(bodyBytes))
	feed, parseErr := parser.Parse(bodyReader)

	if parseErr != nil {
		// Check if it might be a different format than expected
		if strings.Contains(parseErr.Error(), "EOF") || strings.Contains(parseErr.Error(), "no XML") {
			return ValidationResult{URL: url, Status: "invalid", Message: "Not a valid feed format"}
		}
		return ValidationResult{URL: url, Status: "invalid", Message: parseErr.Error()}
	}

	result := ValidationResult{
		URL:       url,
		ItemCount: len(feed.Items),
		Status:    "valid",
	}

	// Check update time if available
	if feed.UpdatedParsed != nil {
		result.LastUpdate = *feed.UpdatedParsed
	} else if len(feed.Items) > 0 && feed.Items[0].PublishedParsed != nil {
		result.LastUpdate = *feed.Items[0].PublishedParsed
	}

	// Add warnings for potential issues but don't mark as invalid
	if len(feed.Items) == 0 {
		result.Message = "Warning: No feed items"
	} else if result.LastUpdate.Before(time.Now().AddDate(0, -6, 0)) {
		result.Message = "Warning: Feed hasn't been updated in over 6 months"
	}

	return result
}

func main() {
	inputFile := "feeds.csv"
	if len(os.Args) > 1 {
		inputFile = os.Args[1]
	}

	file, err := os.Open(inputFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error opening file: %v\n", err)
		os.Exit(1)
	}
	defer file.Close()

	reader := csv.NewReader(file)

	reader.FieldsPerRecord = -1 // Allow varying number of fields
	reader.LazyQuotes = true    // Handle quotes more flexibly
	reader.TrimLeadingSpace = true

	hasHeader := true

	if len(os.Args) > 2 && os.Args[2] == "--no-header" {
		hasHeader = false
	}

	if hasHeader {
		_, err = reader.Read() // Skip header
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error reading header: %v\n", err)
			os.Exit(1)
		}
	}

	var urls []string
	lineNum := 1
	if hasHeader {
		lineNum = 2
	}

	for {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: Skipping line %d due to error: %v\n", lineNum, err)
			lineNum++
			continue
		}
		if len(record) == 0 {
			lineNum++
			continue
		}

		url := record[0]
		if url != "" && !strings.HasPrefix(url, "#") {
			urls = append(urls, url)
		}
		lineNum++
	}

	if len(urls) == 0 {
		fmt.Println("No URLs found to validate")
		os.Exit(0)
	}

	transport := &http.Transport{
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 10,
		IdleConnTimeout:     90 * time.Second,
		DisableCompression:  false,
		DisableKeepAlives:   false,
		// Longer TLS handshake timeout
		TLSHandshakeTimeout: 10 * time.Second,
		// More generous connection timeouts
		ResponseHeaderTimeout: 20 * time.Second,
	}

	client := &http.Client{
		// Don't set client timeout - we're using context timeout instead
		Transport: transport,
	}

	parser := gofeed.NewParser()
	parser.UserAgent = "Mozilla/5.0 (compatible; FeedValidator/1.0)"

	sem := semaphore.NewWeighted(int64(concurrencyLimit))

	var wg sync.WaitGroup
	resultsChan := make(chan ValidationResult, len(urls))

	for _, url := range urls {
		// Acquire semaphore before creating goroutine to ensure controlled concurrency
		if err := sem.Acquire(context.Background(), 1); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to acquire semaphore: %v\n", err)
			continue
		}

		wg.Add(1)

		go func(feedURL string) {
			defer wg.Done()
			defer sem.Release(1)

			result := validateFeed(feedURL, client, parser)
			resultsChan <- result

			statusSymbol := "✅"
			if result.Status == "invalid" {
				statusSymbol = "❌"
			} else if result.Status == "transient" {
				statusSymbol = "⚠️"
			}

			fmt.Printf("%s %s → %s", statusSymbol, result.URL, result.Status)
			if result.Message != "" {
				fmt.Printf(" (%s)", result.Message)
			}
			fmt.Println()
		}(url)
	}

	go func() {
		wg.Wait()
		close(resultsChan)
	}()

	var results []ValidationResult
	for result := range resultsChan {
		results = append(results, result)
	}

	// Generate report
	var valid, invalid, transient, warnings int
	for _, r := range results {
		switch r.Status {
		case "valid":
			valid++
			if r.Message != "" {
				warnings++
			}
		case "invalid":
			invalid++
			fmt.Printf("[Invalid] %s (%s)\n", r.URL, r.Message)
		case "transient":
			transient++
			fmt.Printf("[Transient] %s (%s)\n", r.URL, r.Message)
		}
	}

	total := len(results)
	fmt.Printf("\nResults Summary:\n")
	fmt.Printf("✅ Valid: %d (with %d warnings)\n", valid, warnings)
	fmt.Printf("❌ Invalid: %d\n", invalid)
	fmt.Printf("⚠️ Transient Errors: %d\n", transient)
	fmt.Printf("Total: %d feeds checked\n", total)

	// Consider transient errors as success but log them clearly
	exitCode := 0
	if invalid > 0 {
		exitCode = 1
		// Allow setting environment variable to control exit behavior
		if os.Getenv("IGNORE_INVALID_FEEDS") == "true" {
			exitCode = 0
		}
	}

	// Option to fail on any errors including transient
	if transient > 0 && os.Getenv("FAIL_ON_TRANSIENT") == "true" {
		exitCode = 1
	}

	os.Exit(exitCode)
}
