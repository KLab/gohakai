package main

import (
	"crypto/tls"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"sync"
	"time"

	"golang.org/x/net/http2"
)

const (
	DEFALT_DOMAIN      = "http://localhost:8000"
	DEFALUT_USER_AGENT = "gohakai"
	REMOTE_CONF        = ".gohakai.config.yml"
	HAKAI_BIN_NAME     = "gohakai"
	MODE_NORMAL        = "default"
	MODE_NODE          = "node"
	MODE_NODE_LOCAL    = "node-local"
)

var client http.Client

var Version string
var ExecMode string = MODE_NORMAL
var GitCommit string
var PathCount map[string]uint32
var PathTime map[string]time.Duration
var ok chan bool
var verbose bool
var m sync.Mutex

type Worker struct {
	Client      http.Client
	Config      *Config
	ExVarOffset map[string]int
}

func hakai(c http.Client, config *Config, offset map[string]int) {
	u, err := url.Parse(config.Domain)
	if err != nil {
		log.Fatal(err)
	}

	queryParams := map[string]string{}
	for k, v := range config.QueryParams {
		vv := ReplaceNames(v, offset)
		queryParams[k] = vv
	}

	headers := map[string]string{}
	for k, v := range config.Headers {
		headers[k] = ReplaceNames(v, offset)
	}

	cookieJar, _ := cookiejar.New(nil)
	c.Jar = cookieJar
	attacker := Attacker{
		Client:      &c,
		Url:         u,
		Gzip:        config.Gzip,
		UserAgent:   config.UserAgent,
		QueryParams: &queryParams,
		Headers:     &headers,
		ExVarOffset: offset,
	}
	for _, action := range config.Actions {
		attacker.Action = action
		attacker.Attack()
	}
}

func worker(id int, wg *sync.WaitGroup, limiter chan Worker) {
	for {
		ret := <-limiter
		hakai(ret.Client, ret.Config, ret.ExVarOffset)
		wg.Done()
	}
}

func setupNode(configFile string) {
	var wg sync.WaitGroup
	var i, allProcs int

	for key := range NODES {
		allProcs += NODES[key].Proc
	}

	// scp when nodes option
	for key := range NODES {
		if NODES[key].Host == "localhost" {
			go func(_n Node, o, p int) {
				dumpVars(GOB_FILE, o, _n.Proc, p)
			}(NODES[key], i, allProcs)
		} else {
			wg.Add(1)
			go func(_n Node, o, p int) {
				srcGob, err := ioutil.TempFile(os.TempDir(), fmt.Sprintf("%s.node.%s", GOB_FILE, _n.Host))
				if err != nil {
					log.Println("ioutil.TempFile() error:", err)
					return
				}
				defer os.Remove(srcGob.Name())
				defer wg.Done()

				// scp for gohakai (self-propagation!!)
				// TODO: cofigurable? remote is same architecture, now.
				src := HAKAI_BIN_NAME
				dst := HAKAI_BIN_NAME
				_n.Scp(src, dst)

				// config file
				_n.Scp(configFile, REMOTE_CONF)

				// all vars file
				dst = GOB_FILE
				dumpVars(srcGob.Name(), o, _n.Proc, p)
				_n.Scp(srcGob.Name(), dst)
			}(NODES[key], i, allProcs)
		}

		i += NODES[key].Proc
	}

	wg.Wait()

	fmt.Println("setup node end")
}

func attackNode(configFile string, c chan string, wg *sync.WaitGroup) {
	for key := range NODES {
		wg.Add(1)
		if NODES[key].Host == "localhost" {
			go func(node Node) {
				if err := node.LocalAttack(configFile, c); err != nil {
					log.Println("local attack:", err, node)
				}
			}(NODES[key])
		} else {
			go func(node Node) {
				if err := node.RemoteAttack(c); err != nil {
					log.Println("remote attack:", err, node)
				}
			}(NODES[key])
		}
	}
}

func localMain(loop, maxScenario, maxRequest, totalDuration int, config *Config, stats *Statistics) {
	var wg sync.WaitGroup
	var wgIndicator sync.WaitGroup
	redirectFunc := func(req *http.Request, via []*http.Request) error {
		if len(via) > 10 {
			return fmt.Errorf("%d consecutive requests(redirects)", len(via))
		}
		if len(via) == 0 {
			// No redirects
			return nil
		}
		// mutate the subsequent redirect requests with the first Header
		for key, val := range via[0].Header {
			req.Header[key] = val
			referer := req.Referer()
			checkUrl, err := url.Parse(referer)
			if err != nil {
				log.Printf("url.Parse() Error: %v\n", err)
				return err
			}
			req.URL.RawQuery = checkUrl.RawQuery
		}
		return nil
	}

	if config.HTTPVersion == 2 {
		client = http.Client{
			Transport: &http2.Transport{
				TLSClientConfig: &tls.Config{
					InsecureSkipVerify: false,
				},
			},
			CheckRedirect: redirectFunc,
		}
	} else {
		client = http.Client{
			Transport: &http.Transport{
				MaxIdleConnsPerHost: maxRequest, // default is 2
			},
			Timeout:       time.Duration(config.Timeout) * time.Second, // default is 30
			CheckRedirect: redirectFunc,
		}
	}

	limiter := make(chan Worker, maxRequest)
	stats.MaxRequest = maxRequest
	stats.StartTime = time.Now()

	// exec worker
	for num := 0; num < maxRequest; num++ {
		go worker(num, &wg, limiter)
	}

	// exec indicator & total duration
	ok = make(chan bool)
	indicatorFin := make(chan bool)
	go Indicator(indicatorFin, &wgIndicator)
	wgIndicator.Add(1)
	if totalDuration != 0 {
		go stats.PrintAfterDuration(totalDuration)
	}

	// attack
	for i := 0; i < loop*maxScenario; i++ {
		wg.Add(1)
		offset := map[string]int{}
		for k := range EXVARS {
			offset[k] = EXVARS[k].Offset
			EXVARS[k].Offset += 1
			if EXVARS[k].Offset >= len(EXVARS[k].Value) {
				EXVARS[k].Offset = 0
			}
		}
		w := Worker{Client: client, Config: config, ExVarOffset: offset}
		limiter <- w
	}

	// wait all request & response
	wg.Wait()
	indicatorFin <- true
	wgIndicator.Wait()
}

func clean() {
	if _, err := os.Stat(GOB_FILE); err == nil {
		os.Remove(GOB_FILE)
	}

	if _, err := os.Stat(REMOTE_CONF); err == nil {
		os.Remove(REMOTE_CONF)
	}

	if ExecMode == MODE_NODE {
		os.Remove(HAKAI_BIN_NAME)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "gohakai - Internet Hakai with Go")
	fmt.Fprintf(os.Stderr, "version:%s, id:%s\n\n", Version, GitCommit)
	fmt.Fprintln(os.Stderr, "Usage: gohakai [option] config.yaml")
	flag.PrintDefaults()
	os.Exit(0)
}

func main() {
	if MODE_NODE == os.Getenv("GOHAKAI") {
		ExecMode = MODE_NODE
	} else if MODE_NODE_LOCAL == os.Getenv("GOHAKAI") {
		ExecMode = MODE_NODE_LOCAL
	}

	config := Config{}
	statistics := Statistics{}
	statistics.Config = &config
	var maxScenario, maxRequest, loop, totalDuration int

	// command line option
	flag.IntVar(&maxScenario, "s", 1, "max scenario")
	flag.IntVar(&maxRequest, "c", 0, "max concurrency requests")
	flag.IntVar(&loop, "n", 1, "scenario exec N-loop")
	flag.IntVar(&totalDuration, "d", 0, "total duration")
	flag.BoolVar(&verbose, "verbose", false, "verbose mode")

	flag.Parse()
	args := flag.Args()
	if len(args) < 1 {
		usage()
	}
	configFile := args[0]

	if err := config.Load(configFile); err != nil {
		usage()
	}

	if maxRequest == 0 {
		maxRequest = maxScenario
	}

	PathCount = map[string]uint32{}
	PathTime = map[string]time.Duration{}

	if len(config.Nodes) >= 1 && ExecMode == MODE_NORMAL {
		statChan := make(chan string)
		var statWg sync.WaitGroup

		setupNode(configFile)
		go statistics.Collector(statChan, &statWg)

		attackNode(configFile, statChan, &statWg)
		statWg.Wait()
	} else {
		localMain(loop, maxScenario, maxRequest, totalDuration, &config, &statistics)
		finishTime := time.Now()
		statistics.Delta = finishTime.Sub(statistics.StartTime)
	}

	statistics.Print()

	clean()
}
