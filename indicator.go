package main

import (
	"fmt"
	"sync"
)

var SUCCESS uint32
var FAIL uint32

func Indicator(fin chan bool, wg *sync.WaitGroup) {
	var skip int
	for {
		select {
		case ret := <-ok:
			if MODE_NORMAL != ExecMode {
				continue
			}

			skip += 1
			if ret {
				SUCCESS += 1
				if skip >= 100 {
					fmt.Printf(".")
					skip = 0
				}
			} else {
				FAIL += 1
				fmt.Printf("x")
			}
		case <-fin:
			wg.Done()
		}
	}
}
