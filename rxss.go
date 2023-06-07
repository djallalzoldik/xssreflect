package main

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"unicode/utf8"
)

const (
	maxWorkers = 10
	payload    = `'"%00><h1>akira</h1>`
	fileName   = "injected_urls.txt"
)

func main() {
	scanner := bufio.NewScanner(os.Stdin)
	file, err := os.OpenFile(fileName, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0644)
	if err != nil {
		fmt.Printf("Error opening file: %v\n", err)
		return
	}
	defer file.Close()

	urlCh := make(chan string, maxWorkers)
	var wg sync.WaitGroup

	// Create workers
	for i := 0; i < maxWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for u := range urlCh {
				processURL(u, file)
			}
		}()
	}

	// Read URLs from scanner and send them to the channel
	for scanner.Scan() {
		urlCh <- scanner.Text()
	}
	close(urlCh) // Close the channel to signal workers that no more URLs will be sent

	wg.Wait() // Wait for all workers to finish

	if err := scanner.Err(); err != nil {
		fmt.Fprintln(os.Stderr, "reading standard input:", err)
	}
}

func processURL(u string, file *os.File) {
	parsedURL, err := url.Parse(u)
	if err != nil {
		fmt.Printf("Error parsing URL %s: %v\n", u, err)
		return
	}
	queryValues := parsedURL.Query()
	var queryKeys []string
	for key := range queryValues {
		queryKeys = append(queryKeys, key)
	}
	if len(queryKeys) == 0 {
		fmt.Printf("No query parameters found in URL %s\n", u)
		return
	}

	resp, err := http.Get(u)
	if err != nil {
		fmt.Printf("Error requesting URL %s: %v\n", u, err)
		return
	}
	defer resp.Body.Close()

	buf := bytes.NewBuffer(nil)
	if _, err := io.Copy(buf, resp.Body); err != nil {
		fmt.Printf("Error reading response body from URL %s: %v\n", u, err)
		return
	}

	for _, key := range queryKeys {
		value := queryValues.Get(key)

		if isBase64Encoded(value) {
			decodedValue, err := base64.StdEncoding.DecodeString(value)
			if err != nil {
				fmt.Printf("Error decoding base64-encoded value '%s': %v\n", value, err)
				continue
			}
			value = string(decodedValue)
		}

		if isURLEncoded(value) {
			decodedValue, err := url.QueryUnescape(value)
			if err != nil {
				fmt.Printf("Error decoding URL-encoded value '%s': %v\n", value, err)
				continue
			}
			value = decodedValue
		}

		if strings.Contains(buf.String(), value) {
			injectedValue := value
			if strings.Contains(value, payload) {
				injectedValue = strings.ReplaceAll(value, payload, "")
			} else {
				injectedValue = value + payload
			}
			queryValues.Set(key, injectedValue)
			parsedURL.RawQuery = queryValues.Encode()
			injectedURL := parsedURL.String()

			result := fmt.Sprintf("Query parameter '%s' with value '%s' reflected in response body of %s, replaced with payload %q\n", key, value, parsedURL, payload)
			fmt.Print(result)

			if _, err := file.WriteString(injectedURL + "\n"); err != nil {
				fmt.Printf("Error writing to file: %v\n", err)
			}
		} else {
			result := fmt.Sprintf("Query parameter '%s' with value '%s' not found in response body of %s\n", key, value, parsedURL)
			fmt.Print(result)
		}
	}
}

func isBase64Encoded(s string) bool {
	// Add padding characters to make the length a multiple of 4
	for len(s)%4 != 0 {
		s += "="
	}
	// Check if the string contains any non-base64 characters
	for _, c := range s {
		if !strings.Contains("ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/", string(c)) {
			return false
		}
	}

	// Attempt to decode the string as base64
	decoded, err := base64.StdEncoding.DecodeString(s)
	if err == nil && utf8.Valid(decoded) {
		return true
	}

	return false
}

func isURLEncoded(s string) bool {
	_, err := url.QueryUnescape(s)
	return err == nil
}
