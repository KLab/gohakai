package main

import (
	"bufio"
	"bytes"
	"encoding/gob"
	"io/ioutil"
	"log"
	"math/rand"
	"os"
	"os/user"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"gopkg.in/yaml.v2"
)

const GOB_FILE = ".gohakai.gob"

var CONFIG_ROOT string
var CONSTS map[string]string
var EXVARS map[string]*ExVer
var VARS map[string][]string
var SCANNED_VARS map[string]string
var NODES []Node
var re *regexp.Regexp = regexp.MustCompile("%\\((.+?)\\)%")
var VARS_MUTEX sync.RWMutex

type ExVer struct {
	Value  []string
	Offset int
}

// for remote config
type AllVars struct {
	Vars   map[string][]string
	ExVars map[string]*ExVer
}

type Config struct {
	Domain      string                   `yaml:"domain"`
	UserAgent   string                   `yaml:"user_agent"`
	ShowReport  bool                     `yaml:"show_report"`
	Gzip        bool                     `yaml:"gzip"`
	Timeout     uint16                   `yaml:"timeout"`
	Nodes       []map[string]interface{} `yaml:"nodes"`
	Actions     []map[string]interface{} `yaml:"actions"`
	QueryParams map[string]string        `yaml:"query_params"`
	Consts      map[string]string        `yaml:"consts"`
	ExVars      []map[string]string      `yaml:"exvars"`
	Vars        []map[string]string      `yaml:"vars"`
	Headers     map[string]string        `yaml:"headers"`
	HTTPVersion int                      `yaml:"http_version"`
}

func ReplaceNames(input string, offset map[string]int) string {
	cb := func(s string) string {
		tname := re.FindAllStringSubmatch(s, -1)

		for _, t := range tname {
			if c, ok := CONSTS[t[1]]; ok {
				return c
			}

			if v, ok := VARS[t[1]]; ok {
				return v[rand.Intn(len(v))]
			}

			if e, ok := EXVARS[t[1]]; ok {
				_e := e.Value[offset[t[1]]]
				return _e
			}

			VARS_MUTEX.RLock()
			s, ok := SCANNED_VARS[t[1]]
			VARS_MUTEX.RUnlock()
			if ok {
				return s
			}

			return t[0]
		}

		return tname[0][0]
	}
	ret := re.ReplaceAllStringFunc(input, cb)

	return ret
}

func loadVarsFromFile(filename string) (lines []string) {
	f, err := os.Open(filepath.Join(CONFIG_ROOT, filename))
	if err != nil {
		log.Printf("os.Open() error: %v\n", err)
		os.Exit(-1)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}

	return lines
}

func loadVarsFromGobFile() {
	buf, err := ioutil.ReadFile(GOB_FILE)
	if err != nil {
		log.Println("ioutil.ReadFile() error:", err)
		os.Exit(-1)
	}

	b := bytes.NewBuffer(buf)
	var v AllVars
	dec := gob.NewDecoder(b)
	err = dec.Decode(&v)
	if err != nil {
		log.Fatal("decode:", err)
	}

	EXVARS = v.ExVars
}

// dump gob file
func dumpVars(filename string, offset, procs, allProcs int) {
	var buf bytes.Buffer

	offsets := []int{}
	for i := offset; i < (offset + procs); i++ {
		offsets = append(offsets, i)
	}

	ex := map[string]*ExVer{}
	for key, val := range EXVARS {
		newValue := []string{}
		for _, o := range offsets {
			for i := o; i < len(val.Value); i += allProcs {
				newValue = append(newValue, val.Value[i])
			}
		}

		ex[key] = &ExVer{Value: newValue, Offset: 0}
	}

	// Create an encoder and send a value.
	var v AllVars = AllVars{ExVars: ex, Vars: VARS}
	enc := gob.NewEncoder(&buf)
	err := enc.Encode(v)
	if err != nil {
		log.Fatal("encode:", err)
	}

	ioutil.WriteFile(filename, buf.Bytes(), os.ModePerm)
}

func (c *Config) loadVars() {
	if MODE_NORMAL != ExecMode {
		// when remote execution (from gob file)
		loadVarsFromGobFile()
	} else {
		// when local execution
		for _, v := range c.ExVars {
			EXVARS[v["name"]] = &ExVer{Value: loadVarsFromFile(v["file"])}
		}
		for _, v := range c.Vars {
			VARS[v["name"]] = loadVarsFromFile(v["file"])
		}
	}
}

func (c *Config) loadNodes() {
	for _, v := range c.Nodes {
		// proc
		proc := 1
		tmp, ok := v["proc"]
		if ok {
			proc = tmp.(int)
		}

		if MODE_NORMAL != ExecMode {
			continue
		}

		// user
		var username string
		u, err := user.Current()
		if err != nil {
			panic("user.Current error:")
		}
		username = u.Username

		// host & port
		var hostname string
		var port int = 22
		tmp, ok = v["host"]
		if ok {
			s := tmp.(string)
			ss := strings.Split(s, "@")
			if len(ss) == 2 {
				username = ss[0]
				hostname = ss[1]
			} else {
				hostname = ss[0]
			}
		}
		ss := strings.Split(hostname, ":")
		if len(ss) == 2 {
			hostname = ss[0]
			port, err = strconv.Atoi(ss[1])
			if err != nil {
				panic(v)
			}
		}

		// ssh key
		var sshKeyFile string = "~/.ssh/id_rsa"
		tmp, ok = v["ssh_key"]
		if ok {
			sshKeyFile = tmp.(string)
		}
		sshKeyFile = strings.Replace(sshKeyFile, "~", u.HomeDir, 1)

		n := Node{
			Proc:       proc,
			Port:       port,
			Host:       hostname,
			User:       username,
			SSHKeyFile: sshKeyFile,
		}
		NODES = append(NODES, n)
	}
}

func (c *Config) Load(filename string) error {
	rand.Seed(time.Now().Unix())
	buf, err := ioutil.ReadFile(filename)
	if err != nil {
		return err
	}

	if err = yaml.Unmarshal(buf, &c); err != nil {
		log.Printf("'%s' yaml unmarshal error: %v\n", filename, err)
		return err
	}

	// set default value
	if c.Timeout <= 0 {
		c.Timeout = 1
	}
	if c.UserAgent == "" {
		c.UserAgent = DEFALUT_USER_AGENT
	}
	if c.Domain == "" {
		c.Domain = DEFALT_DOMAIN
	}

	CONFIG_ROOT = filepath.Dir(filename)

	NODES = []Node{}
	VARS = map[string][]string{}
	EXVARS = map[string]*ExVer{}
	CONSTS = c.Consts
	SCANNED_VARS = map[string]string{}

	c.loadVars()
	c.loadNodes()

	return nil
}
