package server

import (
	"errors"
	"fmt"
	"io"
	"log"
	"net"

	"github.com/alexyer/ghost/ghost"
	"github.com/alexyer/ghost/protocol"
	"github.com/golang/protobuf/proto"
)

type client struct {
	Conn       net.Conn
	Server     *Server
	MsgHeader  []byte
	collection *ghost.Collection
}

func newClient(conn net.Conn, s *Server) *client {
	return &client{
		Conn:       conn,
		Server:     s,
		MsgHeader:  make([]byte, MSG_HEADER_SIZE),
		collection: s.storage.GetCollection("main"),
	}
}

func (c *client) String() string {
	return fmt.Sprintf("Client<%s>", c.Conn.LocalAddr())
}

func (c *client) Exec() (reply []byte, err error) {
	var (
		cmd = new(protocol.Command)
	)

	// Read message header
	if err := c.read(c.MsgHeader); err != nil {
		return nil, err
	}

	cmdLen, _ := ghost.ByteArrayToUint64(c.MsgHeader)
	msgBuf := c.Server.bufpool.get(int(cmdLen))

	// Read command to client buffer
	if err := c.read(msgBuf); err != nil {
		return nil, err
	}

	if err := proto.Unmarshal(msgBuf[:cmdLen], cmd); err != nil {
		c.Server.bufpool.put(msgBuf)
		return nil, err
	}

	c.Server.bufpool.put(msgBuf)

	result, err := c.execCmd(cmd)
	return c.encodeReply(result, err)
}

func (c *client) handleCommand() {
	for {
		res, err := c.Exec()

		if err != nil {
			log.Print(err)
			c.Conn.Close()
			return
		}

		replySize := ghost.IntToByteArray(int64(len(res)))

		if _, err := c.Conn.Write(append(replySize, res...)); err != nil {
			c.Conn.Close()
			return
		}
	}
}

func (c *client) execCmd(cmd *protocol.Command) (result []string, err error) {
	switch *cmd.CommandId {
	case protocol.CommandId_PING:
		result, err = c.Ping()
	case protocol.CommandId_SET:
		result, err = c.Set(cmd)
	case protocol.CommandId_GET:
		result, err = c.Get(cmd)
	case protocol.CommandId_DEL:
		result, err = c.Del(cmd)
	case protocol.CommandId_CGET:
		result, err = c.CGet(cmd)
	case protocol.CommandId_CADD:
		result, err = c.CAdd(cmd)
	default:
		err = errors.New("ghost: unknown command")
	}

	return result, err
}

func (c *client) encodeReply(values []string, err error) ([]byte, error) {
	var errMsg string

	if err != nil {
		errMsg = err.Error()
	} else {
		errMsg = ""
	}

	return proto.Marshal(&protocol.Reply{
		Values: values,
		Error:  &errMsg,
	})
}

func (c *client) read(buf []byte) error {
	// TODO(alexyer): Implement proper error handling
	if _, err := c.Conn.Read(buf); err != nil {
		if err != io.EOF {
			return err
		}
	}

	return nil
}
