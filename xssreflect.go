package main

import (
        "bufio"
        "encoding/base64"
        "fmt"
        "io/ioutil"
        "net/http"
        "net/url"
        "os"
        "strings"
        "sync"
        "unicode/utf8"
)

const payload = `'"%00><h1>akira</h1>`

func main() {
        scanner := bufio.NewScanner(os.Stdin)
        var wg sync.WaitGroup
        resultCh := make(chan string)
        file, err := os.Create("injected_urls.txt")
        if err != nil {
                fmt.Printf("Error creating file: %v\n", err)
                return
        }
        defer file.Close()

        for scanner.Scan() {
                u, err := url.Parse(scanner.Text())
                if err != nil {
                        fmt.Printf("Error parsing URL %s: %v\n", scanner.Text(), err)
                        continue
                }
                queryValues := u.Query()
                var queryKeys []string
                for key := range queryValues {
                        queryKeys = append(queryKeys, key)
                }
                if len(queryKeys) == 0 {
                        fmt.Printf("No query parameters found in URL %s\n", scanner.Text())
                        continue
                }

                wg.Add(1)
                go func(u *url.URL, queryKeys []string, resultCh chan<- string) {
                        defer wg.Done()

                        resp, err := http.Get(u.String())
                        if err != nil {
                                resultCh <- fmt.Sprintf("Error requesting URL %s: %v\n", u.String(), err)
                                return
                        }
                        defer resp.Body.Close()

                        body, err := ioutil.ReadAll(resp.Body)
                        if err != nil {
                                resultCh <- fmt.Sprintf("Error reading response body from URL %s: %v\n", u.String(), err)
                                return
                        }

                        for _, key := range queryKeys {
                                value := queryValues.Get(key)

                                if isBase64Encoded(value) {
                                        decodedValue, err := base64.StdEncoding.DecodeString(value)
                                        if err != nil {
                                                resultCh <- fmt.Sprintf("Error decoding base64-encoded value '%s': %v\n", value, err)
                                                continue
                                        }
                                        value = string(decodedValue)
                                }

                                if isURLEncoded(value) {
                                        decodedValue, err := url.QueryUnescape(value)
                                        if err != nil {
                                                resultCh <- fmt.Sprintf("Error decoding URL-encoded value '%s': %v\n", value, err)
                                                continue
                                        }
                                        value = decodedValue
                                }

                                if strings.Contains(string(body), value) {
                                        injectedValue := value
                                        if strings.Contains(value, payload) {
                                                injectedValue = strings.ReplaceAll(value, payload, "")
                                        } else {
                                                injectedValue = value + payload
                                        }
                                        queryValues.Set(key, injectedValue)
                                        u.RawQuery = queryValues.Encode()
                                        injectedURL := u.String()

                                        resultCh <- fmt.Sprintf("Query parameter '%s' with value '%s' reflected in response body of %s, replaced with payload %q\n", key, value, u.String(), payload)

                                        if _, err := file.WriteString(injectedURL + "\n"); err != nil {
                                                fmt.Printf("Error writing to file: %v\n", err)
                                        }
                                } else {
                                        resultCh <- fmt.Sprintf("Query parameter '%s' with value '%s' not found in response body of %s\n", key, value, u.String())
                                }
                        }
        }(u, queryKeys, resultCh)
}

go func() {
        wg.Wait()
        close(resultCh)
}()

for res := range resultCh {
        fmt.Print(res)
}

if err := scanner.Err(); err != nil {
        fmt.Fprintln(os.Stderr, "reading standard input:", err)
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
