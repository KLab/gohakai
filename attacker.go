package main

import (
	"compress/gzip"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"
)

type Attacker struct {
	Url         *url.URL
	Client      *http.Client
	Action      map[string]interface{}
	UserAgent   string
	Gzip        bool
	QueryParams *map[string]string
	ExVarOffset map[string]int
	sync.RWMutex
}

func (atk *Attacker) makeRequest() (req *http.Request, err error) {
	checkPath := ReplaceNames(atk.Action["path"].(string), atk.ExVarOffset)
	checkUrl, err := url.Parse(checkPath)
	if err != nil {
		log.Printf("url.Parse() Error: %v\n", err)
		return nil, err
	}

	atk.Url.Path = checkUrl.Path

	method, ret := atk.Action["method"]
	if ret != true {
		method = "GET"
	}

	var content io.Reader
	values := url.Values{}
	postParams, retPostParams := atk.Action["post_params"]
	if method == "POST" && retPostParams {
		for k, v := range postParams.(map[interface{}]interface{}) {
			values.Add(k.(string), ReplaceNames(v.(string), atk.ExVarOffset))
		}
		content = strings.NewReader(values.Encode())
	} else {
		if _content, ret := atk.Action["content"]; ret {
			content = strings.NewReader(ReplaceNames(_content.(string), atk.ExVarOffset))
		}
	}

	req, err = http.NewRequest(method.(string), atk.Url.String(), content)
	if err != nil {
		log.Printf("NewRequest Error: %v\n", err)
		return nil, err
	}
	contentType, ret := atk.Action["content_type"]
	if ret == true {
		req.Header.Set("Content-Type", contentType.(string))
	} else if method == "POST" && retPostParams {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}

	if atk.Gzip {
		req.Header.Set("Accept-Encoding", "gzip")
	} else {
		req.Header.Set("Accept-Encoding", "")
	}

	values = url.Values{}
	for k, v := range checkUrl.Query() {
		values.Add(k, v[0])
	}
	for k, v := range *atk.QueryParams {
		values.Add(k, v)
	}
	req.URL.RawQuery = values.Encode()

	req.Header.Set("User-Agent", atk.UserAgent)
	return req, err
}

func wrapRegexp(s string) interface{} {
	return regexp.MustCompile(s)
}

func (atk *Attacker) Attack() {
	req, err := atk.makeRequest()
	if err != nil {
		ok <- false
		return
	}

	if verbose {
		if len(req.URL.RawQuery) >= 1 {
			log.Printf("%s %s?%s\n", req.Method, req.URL.Path, req.URL.RawQuery)
		} else {
			log.Printf("%s %s\n", req.Method, req.URL.Path)
		}
	}

	t0 := time.Now()
	res, err := atk.Client.Do(req)
	if err != nil {
		log.Printf("request error: %v\n", err)
		ok <- false
		return
	}
	defer res.Body.Close()

	t1 := time.Now()
	diffTime := t1.Sub(t0)

	validRes := true
	atk.RLock()
	_scan, ret := atk.Action["scan"]
	atk.RUnlock()
	if ret {
		// check body text
		var reader io.ReadCloser
		switch res.Header.Get("Content-Encoding") {
		case "gzip", "deflate":
			reader, _ = gzip.NewReader(res.Body)
			defer reader.Close()
		default:
			reader = res.Body
		}
		body, _ := ioutil.ReadAll(reader)

		// memoization
		var scan *regexp.Regexp
		_s, _ok := _scan.(string)
		if _ok {
			atk.Lock()
			atk.Action["scan"] = wrapRegexp(_s)
			atk.Unlock()
		}
		atk.RLock()
		scan = atk.Action["scan"].(*regexp.Regexp)
		atk.RUnlock()

		if scan.Match(body) {
			names := scan.SubexpNames()
			for _, tname := range scan.FindAllStringSubmatch(string(body), -1) {
				for i, name := range tname[1:] {
					VARS_MUTEX.Lock()
					SCANNED_VARS[names[i+1]] = name
					VARS_MUTEX.Unlock()
				}
			}
		} else {
			validRes = false
			log.Println(atk.Url)
			fmt.Print(string(body))
		}
	} else {
		if _, err := ioutil.ReadAll(res.Body); err != nil {
			log.Println(err)
		}
	}

	if verbose {
		log.Println(diffTime, res.StatusCode, res.ContentLength)
	}

	m.Lock()
	PathCount[atk.Url.Path] += 1
	PathTime[atk.Url.Path] += diffTime
	m.Unlock()

	if validRes && res.StatusCode/10 == 20 {
		ok <- true
	} else {
		ok <- false
	}
}
