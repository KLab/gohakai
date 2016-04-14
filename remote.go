package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"strings"

	"golang.org/x/crypto/ssh"
)

type Node struct {
	Proc       int
	Port       int
	Host       string
	User       string
	SSHKeyFile string
	Session    *ssh.Session
}

func (n *Node) NewSSHSession() (session *ssh.Session, err error) {
	pkey, err := ioutil.ReadFile(n.SSHKeyFile)
	if err != nil {
		log.Println("ioutil.ReadFile(sshkey):", err)
		return session, err
	}

	s, err := ssh.ParsePrivateKey(pkey)
	if err != nil {
		log.Println("ssh.ParsePrivateKey():", err)
		return session, err
	}

	config := &ssh.ClientConfig{
		User: n.User,
		Auth: []ssh.AuthMethod{
			ssh.PublicKeys(s),
		},
	}

	host := fmt.Sprintf("%s:%d", n.Host, n.Port)
	client, err := ssh.Dial("tcp", host, config)
	if err != nil {
		log.Println("ssh.Dial:", err)
		return session, err
	}

	session, err = client.NewSession()
	if err != nil {
		log.Println("cli.NewSession():", err)
		return session, err
	}

	return session, err
}

// scp for self exec file and config.yml
func (n *Node) Scp(src, dst string) (err error) {
	n.Session, err = n.NewSSHSession()
	if err != nil {
		log.Println("new ssh session error:", err)
		return err
	}
	defer n.Session.Close()

	go func() {
		w, _ := n.Session.StdinPipe()
		defer w.Close()
		src, _ := os.Open(src)
		srcStat, _ := src.Stat()
		fmt.Fprintln(w, "C0755", srcStat.Size(), dst)
		io.Copy(w, src)
		fmt.Fprint(w, "\x00")
	}()

	var b bytes.Buffer
	n.Session.Stdout = &b
	if err := n.Session.Run("/usr/bin/scp -tr ./"); err != nil {
		log.Println("session.Run() error:", err)
		return err
	}

	return
}

// return []string{"-f 1", "-s 1", ...}
// skip -f option
func rebuildArgs() (ret []string) {
	args := []string{"s", "c", "n", "d"}
	for _, v := range args {
		if f := flag.Lookup(v); f != nil {
			ret = append(ret, fmt.Sprintf("-%s", v))
			ret = append(ret, fmt.Sprintf("%s", f.Value))
		}
	}

	return ret
}

func (n *Node) LocalAttack(configFile string, c chan string) (err error) {
	args := rebuildArgs()
	args = append(args, "-f")
	args = append(args, fmt.Sprintf("%d", n.Proc))
	args = append(args, configFile)
	cmd := exec.Command(fmt.Sprintf("./%s", HAKAI_BIN_NAME), args...)
	cmd.Env = []string{fmt.Sprintf("GOHAKAI=%s", MODE_NODE_LOCAL)}

	out, err := cmd.CombinedOutput()
	if err != nil {
		return err
	}

	c <- string(out)

	return err
}

func (n *Node) RemoteAttack(c chan string) (err error) {
	rawCmd := strings.Join(rebuildArgs(), " ")
	command := fmt.Sprintf("GOHAKAI=%s ./%s %s -f %d %s",
		MODE_NODE, HAKAI_BIN_NAME, rawCmd, n.Proc, REMOTE_CONF)

	n.Session, _ = n.NewSSHSession()
	defer n.Session.Close()

	var b bytes.Buffer
	n.Session.Stdout = &b
	if err := n.Session.Run(command); err != nil {
		log.Println("attack error:", err)
		return err
	}

	if len(b.String()) == 0 {
		return errors.New("non result error!!")
	}

	c <- b.String()

	return err
}
