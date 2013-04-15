/**
 * This is a webserver which generates some text indicating that the
 * response was generated a certain time, and gives the last modified
 * "date" of the content. That content is "generated" every X seconds,
 * which is defined by the granularity_in_seconds constant.
 *
 * This gives the impression that the content is being newly generated
 * every X seconds, and so means you can perform validation tests with
 * it.
 */

package main

import (
    "fmt"
    "net/http"
    "os"
    "sort"
    "strconv"
    "strings"
    "time"
)

const granularity_in_seconds = 15
const help_text = `
USAGE: You can go to any URL and get some basic content.

If the following components are present in the path URL, then you will
get some additional behaviour:
  /datemod/ -> Sets a Date-Modified header and performs date resource validation.
  /etag/ -> Sets an E-Tag header and performs E-Tag resource validation.
  /headers/ -> Includes the request headers in the content response.
  /static/ -> Uses a fixed timestamp rather than an updating one.
  
You can combine the path components too:
  /resource/headers/static/blahblah/etag/anythingyoulike/
`

type ByteWriter struct {
    buf []byte
    pos int
}

func Buffy() *ByteWriter {
    res := new(ByteWriter)
    res.buf = make([]byte, 4096) // 4K should be enough for anyone!
    res.pos = 0
    return res
}

func (s *ByteWriter) Write(bs []byte) (n int, err error) {
    n = len(bs)
    copy(s.buf[s.pos:s.pos+n], bs)
    s.pos += n
    return
}

func (b *ByteWriter) AsString() string {
    return string(b.buf[0:b.pos]);
}

func handle(w http.ResponseWriter, r *http.Request) {

    respond := func (code int, message string) {
        timestamp := time.Now().Format("[2006/01/02 15:04:05]")
        fmt.Println(timestamp, r.Method, r.URL, "->", code)
        if (message != "") {
            http.Error(w, message, code)
        }
    }

    // Only GET allowed.
    if r.Method != "GET" {
        respond(405, "Only GET requests supported")
        return
    }
    
    // Determine what things we want to do for our response.
    print_headers := strings.Index(r.URL.Path, "/headers/") != -1
    use_etags := strings.Index(r.URL.Path, "/etag/") != -1
    use_lastmod := strings.Index(r.URL.Path, "/lastmod/") != -1
    be_static := strings.Index(r.URL.Path, "/static/") != -1
    
    // This is the content.
    now := time.Now().UTC()
    now_header := now.Format(time.RFC1123Z)
    seconds := ((now.Second() / granularity_in_seconds) * granularity_in_seconds)
    then := time.Date(now.Year(), now.Month(), now.Day(), now.Hour(), now.Minute(), seconds, 0, now.Location())
    if be_static {
        then = time.Date(2012, 12, 20, 20, 12, 12, 0, now.Location())
    }
    then_header := then.Format(time.RFC1123Z)

    // Custom headers for the response.
    headers := make(map[string]string)
        
    // E-Tag check, based on the method "validate_etags" in cptools.py
    // in CherryPy's module.
    if use_etags {
    
        // Generate the E-Tag.
        etag := then.Format("2006-01-02,15:04:05")
        etag_enc := "\"" + etag + "\""
        
        // Note: Not bothering with multiple headers at the moment.
        
        // Check If-Match header.
        etag_if_match := r.Header.Get("If-Match")
        if (len(etag_if_match) > 0) && 
            !(etag_enc == "\"*\"" || etag_enc == etag_if_match) {
            respond(412, "If-Match failed: ETag did not match")
            return
        }
        
        // Check If-None-Match header.
        etag_if_none_match := r.Header.Get("If-None-Match")
        if (len(etag_if_none_match) > 0) && (
            etag_enc == "\"*\"" || etag_enc == etag_if_none_match) {
            respond(304, "Content matches on If-None-Match")
            return
        }
        
        headers["E-Tag"] = etag_enc
    }

    // Last modified check, based on the method "validate_since" in
    // cptools.py in CherryPy's module.
    if use_lastmod {
    
        // Check If-Unmodified-Since header.
        unmod_since := r.Header.Get("If-Unmodified-Since")
        if (len(unmod_since) > 0) && (unmod_since != then_header) {
            respond(412, "Document has been modified")
            return
        }

        // Check If-Modified-Since header.
        mod_since := r.Header.Get("If-Modified-Since")
        if (len(mod_since) > 0) && (mod_since != then_header) {
            respond(304, "Document has not been modified")
            return
        }
        
        headers["Last-Modified"] = then_header

    }
    
    w.Header().Set("Content-Type", "text/plain; charset=utf-8")
    if r.URL.Path == "/" {
        w.Header().Set("Content-Length", strconv.Itoa(len(help_text)))
        fmt.Fprint(w, help_text)
        return
    }

    /**
     * Response-writing time! We write in a buffer because we want to
     * determine and set the Content-Length, as Go's libraries won't do
     * it automatically.
     */    
    b := Buffy()
    
    fmt.Fprintln(b, "Content date:", then_header)
    fmt.Fprintln(b, "Generated:   ", now_header)

    if (print_headers) {
        fmt.Fprintln(b, "\nREQUEST HEADERS:")
        keys := make([]string, 0, len(r.Header))
        for k := range r.Header {
            keys = append(keys, k)
        }
        sort.Strings(keys)
        for _, key := range keys {
            fmt.Fprintf(b, "  %v: %v\n", key, r.Header[key][0])
        }
    }
    
    // Copy headers into the response.
    for key, val := range headers {
        w.Header().Set(key, val)
    }
    
    // Write out buffered content.
    content := b.AsString()
    w.Header().Set("Content-Length", strconv.Itoa(len(content)))
    respond(200, "")
    fmt.Fprint(w, content)
    return
}

func main() {
    if len(os.Args) != 2 {
        fmt.Fprintln(os.Stderr, "You must pass a single argument of the address to listen on.")
        fmt.Fprintln(os.Stderr, "  (e.g. \"localhost:4000\")")
        return
    }
    
    http.HandleFunc("/", handle)
    fmt.Println("Listening and serving on", os.Args[1])
    serve_err := http.ListenAndServe(os.Args[1], nil)
    if serve_err != nil {
        fmt.Fprintln(os.Stderr, "ERROR:", serve_err)
    }
}
