package mysql

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"

	"github.com/IceFireDB/IceFireDB/IceFireDB-SQLProxy/pkg/config"
	"github.com/IceFireDB/IceFireDB/IceFireDB-SQLProxy/pkg/mysql/client"
	"github.com/IceFireDB/IceFireDB/IceFireDB-SQLProxy/pkg/mysql/server"
	"github.com/IceFireDB/IceFireDB/IceFireDB-SQLProxy/utils"
	"github.com/sirupsen/logrus"
)

func Run(ctx context.Context) (err error) {
	ms := newMysqlProxy()
	ms.ctx = ctx
	ms.closed.Store(true)

	if err = ms.initClientPool(); err != nil {
		return fmt.Errorf("initClientPool error: %v", err)
	}
	ln, err := net.Listen("tcp4", config.Get().Server.Addr)
	if err != nil {
		logrus.Errorf("mysql%v", err)
		return
	}
	utils.GoWithRecover(func() {
		if <-ctx.Done(); true {
			_ = ln.Close()
			ms.closed.Store(true)
		}
	}, nil)

	ms.closed.Store(false)
	logrus.Infof("%s\n", config.Get().Server.Addr)
	// p2p
	if config.Get().P2P.Enable {
		initP2P(ms)
	}
	for {
		conn, err := ln.Accept()
		if err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, net.ErrClosed) {
				continue
			}
			panic(err)
		}
		go ms.onConn(conn)
	}
	return
}

func newMysqlProxy() *mysqlProxy {
	p := &mysqlProxy{}
	p.server = server.NewDefaultServer()
	p.credential = server.NewInMemoryProvider()
	for _, info := range config.Get().UserList {
		p.credential.AddUser(info.User, info.Password)
	}
	return p
}

func (m *mysqlProxy) initClientPool() error {
	mc := config.Get().Mysql

	// Initialize admin pool
	adminPool, err := client.NewPool(
		logrus.Infof,
		mc.Admin.MinAlive,
		mc.Admin.MaxAlive,
		mc.Admin.MaxIdle,
		mc.Admin.Addr,
		mc.Admin.User,
		mc.Admin.Password,
		mc.Admin.DBName,
	)
	if err != nil {
		return fmt.Errorf("failed to create admin pool: %v", err)
	}
	m.adminPool = adminPool

	// Initialize readonly pool if configured
	readonlyPool, err := client.NewPool(
		logrus.Infof,
		mc.Readonly.MinAlive,
		mc.Readonly.MaxAlive,
		mc.Readonly.MaxIdle,
		mc.Readonly.Addr,
		mc.Readonly.User,
		mc.Readonly.Password,
		mc.Readonly.DBName,
	)
	if err != nil {
		return fmt.Errorf("failed to create readonly pool: %v", err)
	}
	m.readonlyPool = readonlyPool

	return nil
}
