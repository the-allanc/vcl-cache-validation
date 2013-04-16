package main

import (
    "net/http"
    "net/http/httputil"
    "strings"
    "testing"
    "time"
)

const granularity_in_seconds = 15
const target_server = "http://localhost:20752"
var client = http.Client{}

/**
 * We run all tests in parallel.
 */

func MakeReq(t *testing.T, url string) *http.Request {
    // We start execution in the first five seconds of an execution
    // loop.
    //for {
    //    now := time.Now().UTC()
    //    if (now.Second() % granularity_in_seconds) < 5 {break}
    //    time.Sleep(1 * time.Second)
    //}
    resp, err := http.NewRequest("GET", target_server + url, nil)
    if (err != nil) {
        t.Fatal("Error generating request.", err)
    }
    return resp
}

func DoReq(t *testing.T, req *http.Request, expected_code int) *http.Response {
    //t.Log("REQUEST:")
    b, err := httputil.DumpRequestOut(req, true)
    if err != nil {t.Fatal("Error dumping out request.", err)}
    t.Log("    " + strings.TrimSpace(string(b)))
    
    resp, err := client.Do(req)
    if err != nil {t.Fatal("Error retrieving response.", err)}

    //t.Log("RESPONSE:")
    b, err = httputil.DumpResponse(resp, true)
    if err != nil {t.Fatal("Error dumping out response.", err)}
    t.Log("    " + strings.TrimSpace(string(b)))

    if resp.StatusCode != expected_code {
        t.Fatal("Expected status code", expected_code, "response code, but got", resp.StatusCode)
    }
    return resp
}

func GetBody(t *testing.T, resp *http.Response) string {
    buffer := make([]byte, 16384) // 16K should be enough for anyone!
    size, err := resp.Body.Read(buffer)
    defer resp.Body.Close()
    if err != nil {
        t.Fatal("Error extracting body.", err)
    }
    return strings.TrimSpace(string(buffer[0:size]))
}

func CompareBodies(t *testing.T, body1 string, body2 string, same bool, identical bool) {
    line1 := strings.Split(body1, "\n")[0]
    line2 := strings.Split(body2, "\n")[0]
    if same && (line1 != line2) {
        //t.Log("First req:")
        //t.Log(line1)
        //t.Log("Second req:")
        //t.Log(line2)
        t.Fatal("First lines of requests are different, expected to be same.")    
    } else if !same && (line1 == line2) {
        //t.Log("First line of request:")
        //t.Log(line1)
        t.Fatal("First line of request bodies are identical, expected differences.")
    }
    if identical && (body1 != body2) {
        //t.Log("First body:")
        //t.Log(body1)
        //t.Log("Second body:")
        //t.Log(body2)
        t.Fatal("Request bodies are not the same, expected to be identical.")
    } else if !identical && (body1 == body2) {
        //t.Log("Body:")
        //t.Log(body1)
        t.Fatal("Request bodies are identical, expected differences.")
    }
}

type Validator interface {
    Build(t *testing.T, r *http.Response) (*http.Request, *http.Request)
}

type Updater interface {
    Update(t *testing.T, r *http.Request)
}

type ETagV struct {}

func (*ETagV) Build(t *testing.T, r *http.Response) (*http.Request, *http.Request) {
    modded := MakeReq(t, r.Request.URL.Path)
    same := MakeReq(t, r.Request.URL.Path)
    modded.Header.Add("If-None-Match", r.Header.Get("ETag"))
    same.Header.Add("If-Match", r.Header.Get("ETag"))
    return modded, same
}

type DateModV struct {}

func (*DateModV) Build(t *testing.T, r *http.Response) (*http.Request, *http.Request) {
    modded := MakeReq(t, r.Request.URL.Path)
    same := MakeReq(t, r.Request.URL.Path)
    modded.Header.Add("If-Modified-Since", r.Header.Get("Last-Modified"))
    same.Header.Add("If-Unmodified-Since", r.Header.Get("Last-Modified"))
    return modded, same
}

type ClockU struct {}
func (*ClockU) Update(t *testing.T, req *http.Request) {
    snooze()
    r := MakeReq(t, req.URL.Path)
    r.Method = "PUT"
    DoReq(t, r, 204)
}

type PeriodicU struct {}
func (*PeriodicU) Update(t *testing.T, req *http.Request) {
    for i:=0; i < granularity_in_seconds; i++ {
        snooze()
    }
}

func DoValidationTest(t *testing.T, path string, v Validator, u Updater) {
    t.Parallel()
    
    t.Log("Getting original request.")
    req := MakeReq(t, path)
    resp := DoReq(t, req, 200)
    body := GetBody(t, resp)
    
    // Add the validator, and do the test again. It shouldn't have
    // changed.
    modded, same := v.Build(t, resp)
    t.Log("Checking original document hasn't been modified.")
    sresp := DoReq(t, same, 200)
    DoReq(t, modded, 304)
    sbody := GetBody(t, sresp)
    CompareBodies(t, body, sbody, true, true)
    
    // Update the document.
    if (u == nil) {return;}
    t.Log("Updating document.")
    u.Update(t, req)
    
    // Now if we try again, the requests should behave differently.
    t.Log("Checking document is different.")
    mresp := DoReq(t, modded, 200)
    DoReq(t, same, 412)
    mbody := GetBody(t, mresp)
    CompareBodies(t, body, mbody, false, false)
}

func snooze() {
    time.Sleep(time.Second)
}

func TestBasicMissingDoc(t *testing.T) {
    t.Parallel()
    req := MakeReq(t, "/gosomewhere/notexpected/")
    DoReq(t, req, 410)
}

func TestStaticDoc(t *testing.T) {
    t.Parallel()
    req := MakeReq(t, "/static/ourtestdoc/")
    resp1 := DoReq(t, req, 200)
    content1 := GetBody(t, resp1)
    
    // Get the document again - it should be identical.
    snooze()
    resp2 := DoReq(t, req, 200)
    content2 := GetBody(t, resp2)
    
    // First line should be the same - the date is static.
    // The generation line should differ though.
    CompareBodies(t, content1, content2, true, false)
}

func TestStaticEtags(t *testing.T) {
    DoValidationTest(t, "/static/etag/functest/", &ETagV{}, nil)
}

func TestStaticModded(t *testing.T) {
    DoValidationTest(t, "/static/lastmod/functest/", &DateModV{}, nil)
}

func TestPeriodicEtags(t *testing.T) {
    DoValidationTest(t, "/periodic/etag/functest/", &ETagV{}, &PeriodicU{})
}

func TestPeriodicModded(t *testing.T) {
    DoValidationTest(t, "/periodic/lastmod/functest/", &DateModV{}, &PeriodicU{})
}

func TestClockEtags(t *testing.T) {
    DoValidationTest(t, "/clock/etag/functest/", &ETagV{}, &ClockU{})
}

func TestClockModded(t *testing.T) {
    DoValidationTest(t, "/clock/lastmod/functest/", &DateModV{}, &ClockU{})
}

// XXX: HEADER TEST
