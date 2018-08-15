package cssh

import (
	"golang.org/x/crypto/ssh"
	"io/ioutil"
	"errors"
)

type Cssh struct {
	Ip          string
	Port        string
	User        string
	KeyFilePath string
	sess *ssh.Session
}

func (c *Cssh) Connect() (err error) {
	var buf []byte
	if buf, err = ioutil.ReadFile(c.KeyFilePath); err != nil {
		return
	}
	var key ssh.Signer
	if key, err = ssh.ParsePrivateKey(buf); err != nil {
		return
	}
	conf := &ssh.ClientConfig{
		User: c.User,
		Auth: []ssh.AuthMethod{
			ssh.PublicKeys(key),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}
	var conn *ssh.Client
	if conn, err = ssh.Dial("tcp", c.Ip+":"+c.Port, conf); err != nil {
		return
	}
	c.sess, err = conn.NewSession()
	return
}

func (c *Cssh) Run(cmd *string) (response []byte, err error) {
	if c.sess != nil {
		response, err = c.sess.Output(*cmd)
	} else {
		err = errors.New("must Connect() first")
	}
	return
}


func (c *Cssh) RunotWait(cmd *string) (err error) {
	if c.sess != nil {
		err = c.sess.Run(*cmd)
	} else {
		err = errors.New("must Connect() first")
	}
	return
}


func (c *Cssh) FRun(dst [][]byte, cmds *[]string) (err error) {
	if c.sess != nil {
		for _, cmd := range *cmds {
			var d []byte
			if d, err = c.sess.Output(cmd); err != nil {
				return
			} else {
				dst = append(dst, d)
			}
		}
	} else {
		err = errors.New("must Connect() first")
	}
	return
}

func (c *Cssh) Close() error {
	return c.sess.Close()
}
