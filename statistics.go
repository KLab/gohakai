package main

import (
	"bytes"
	"encoding/gob"
	"fmt"
	"log"
	"os"
	"sort"
	"sync"
	"time"
)

type Statistics struct {
	MaxRequest int
	StartTime  time.Time
	Delta      time.Duration
	Config     *Config
}

type NodeStats struct {
	Success     uint32
	Fail        uint32
	Concurrency int
	Time        time.Duration
	PathCount   map[string]uint32
	PathTime    map[string]time.Duration
}

type AvarageTimeByPath struct {
	Path string
	Time float64
}
type AvarageTimeStats []AvarageTimeByPath

func (s AvarageTimeStats) Len() int {
	return len(s)
}
func (s AvarageTimeStats) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}
func (s AvarageTimeStats) Less(i, j int) bool {
	return s[i].Time < s[j].Time
}

func (s *Statistics) PrintAfterDuration(duration int) {
	time.Sleep(time.Duration(duration) * time.Second)
	s.Print()
	os.Exit(0)
}

func (s *Statistics) printGob() {
	delta := s.Delta

	var buf bytes.Buffer
	var n NodeStats = NodeStats{
		Success:     SUCCESS,
		Fail:        FAIL,
		Concurrency: s.MaxRequest,
		Time:        delta,
		PathCount:   PathCount,
		PathTime:    PathTime,
	}
	enc := gob.NewEncoder(&buf)
	err := enc.Encode(n)
	if err != nil {
		log.Fatal("encode:", err)
	}

	fmt.Print(buf.String())
}

func (s *Statistics) printHumanReadable() {
	delta := s.Delta
	nreq := SUCCESS + FAIL
	rps := float64(nreq) / float64(delta.Seconds())

	fmt.Printf("\nrequest count:%d, concurrency:%d, time:%.5f[s], %f[req/s]\n",
		nreq, s.MaxRequest, delta.Seconds(), rps)
	fmt.Printf("SUCCESS %d\n", SUCCESS)
	fmt.Printf("FAILED %d\n", FAIL)

	var avgTimeByPath map[string]float64 = map[string]float64{}
	var totalCount uint32
	var totalTime time.Duration
	for path, cnt := range PathCount {
		totalTime += PathTime[path]
		totalCount += cnt
		avgTimeByPath[path] += PathTime[path].Seconds() / float64(cnt)
	}
	fmt.Printf("Average response time[ms]: %v\n",
		1000.*totalTime.Seconds()/float64(totalCount))

	if s.Config.ShowReport {
		var stats AvarageTimeStats = []AvarageTimeByPath{}

		fmt.Printf("Average response time for each path (order by longest) [ms]:\n")
		for path, time := range avgTimeByPath {
			stats = append(stats, AvarageTimeByPath{Path: path, Time: time})
		}
		sort.Sort(sort.Reverse(stats))
		for i := 0; i < len(stats); i++ {
			fmt.Printf("%.3f : %s\n", stats[i].Time*1000., stats[i].Path)
		}
	}
}

func (s *Statistics) Print() {
	if MODE_NORMAL != ExecMode {
		s.printGob()
	} else {
		s.printHumanReadable()
	}
}

func parseResultGob(s string) (result NodeStats, err error) {
	b := bytes.NewBufferString(s)

	dec := gob.NewDecoder(b)
	err = dec.Decode(&result)
	if err != nil {
		log.Fatal("decode:", err)
	}

	return
}

func (s *Statistics) Collector(c chan string, wg *sync.WaitGroup) {
	s.Delta = time.Duration(0)
	for {
		ret := <-c
		n, _ := parseResultGob(ret)
		SUCCESS += n.Success
		FAIL += n.Fail
		s.MaxRequest += n.Concurrency
		s.Delta += n.Time
		for path, cnt := range n.PathCount {
			PathTime[path] += n.PathTime[path]
			PathCount[path] += cnt
		}
		wg.Done()
	}
}
