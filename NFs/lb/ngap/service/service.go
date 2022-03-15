package service

import (
	"crypto/rand"
	"encoding/hex"
	"io"
	"net"
	"sync"
	"syscall"

	"git.cs.nctu.edu.tw/calee/sctp"

	"loadbalance/context"
	"loadbalance/logger"

	"github.com/free5gc/ngap"
)

type NGAPHandler struct {
	HandleMessage      func(conn net.Conn, msg []byte)
	HandleNotification func(conn net.Conn, notification sctp.Notification)
}

const readBufSize uint32 = 8192

// set default read timeout to 2 seconds
var readTimeout syscall.Timeval = syscall.Timeval{Sec: 2, Usec: 0}

var (
	sctpListener *sctp.SCTPListener
	connections  sync.Map
	amfConn      *sctp.SCTPConn
	newConn      *sctp.SCTPConn
	err          error
)

var sctpConfig sctp.SocketConfig = sctp.SocketConfig{
	InitMsg:   sctp.InitMsg{NumOstreams: 3, MaxInstreams: 5, MaxAttempts: 2, MaxInitTimeout: 2},
	RtoInfo:   &sctp.RtoInfo{SrtoAssocID: 0, SrtoInitial: 500, SrtoMax: 1500, StroMin: 100},
	AssocInfo: &sctp.AssocInfo{AsocMaxRxt: 4},
}

func Run(addresses []string, port int) {
	ips := []net.IPAddr{}

	for _, addr := range addresses {
		if netAddr, err := net.ResolveIPAddr("ip", addr); err != nil {
			logger.NgapLog.Errorf("Error resolving address '%s': %v\n", addr, err)
		} else {
			logger.NgapLog.Debugf("Resolved address '%s' to %s\n", addr, netAddr)
			ips = append(ips, *netAddr)
		}
	}

	addr := &sctp.SCTPAddr{
		IPAddrs: ips,
		Port:    port,
	}

	go listenAndServe(addr)
}

func listenAndServe(addr *sctp.SCTPAddr) {
	if listener, err := sctpConfig.Listen("sctp", addr); err != nil {
		logger.NgapLog.Errorf("Failed to listen: %+v", err)
		return
	} else {
		sctpListener = listener
	}

	logger.NgapLog.Infof("Listen on %s", sctpListener.Addr())

	for {
		newConn, err = sctpListener.AcceptSCTP()
		if err != nil {
			switch err {
			case syscall.EINTR, syscall.EAGAIN:
				logger.NgapLog.Debugf("AcceptSCTP: %+v", err)
			default:
				logger.NgapLog.Errorf("Failed to accept: %+v", err)
			}
			continue
		}

		var info *sctp.SndRcvInfo
		if infoTmp, err := newConn.GetDefaultSentParam(); err != nil {
			logger.NgapLog.Errorf("Get default sent param error: %+v, accept failed", err)
			if err = newConn.Close(); err != nil {
				logger.NgapLog.Errorf("Close error: %+v", err)
			}
			continue
		} else {
			info = infoTmp
			logger.NgapLog.Debugf("Get default sent param[value: %+v]", info)
		}

		info.PPID = ngap.PPID
		if err := newConn.SetDefaultSentParam(info); err != nil {
			logger.NgapLog.Errorf("Set default sent param error: %+v, accept failed", err)
			if err = newConn.Close(); err != nil {
				logger.NgapLog.Errorf("Close error: %+v", err)
			}
			continue
		} else {
			logger.NgapLog.Debugf("Set default sent param[value: %+v]", info)
		}

		events := sctp.SCTP_EVENT_DATA_IO | sctp.SCTP_EVENT_SHUTDOWN | sctp.SCTP_EVENT_ASSOCIATION
		if err := newConn.SubscribeEvents(events); err != nil {
			logger.NgapLog.Errorf("Failed to accept: %+v", err)
			if err = newConn.Close(); err != nil {
				logger.NgapLog.Errorf("Close error: %+v", err)
			}
			continue
		} else {
			logger.NgapLog.Debugln("Subscribe SCTP event[DATA_IO, SHUTDOWN_EVENT, ASSOCIATION_CHANGE]")
		}

		if err := newConn.SetReadBuffer(int(readBufSize)); err != nil {
			logger.NgapLog.Errorf("Set read buffer error: %+v, accept failed", err)
			if err = newConn.Close(); err != nil {
				logger.NgapLog.Errorf("Close error: %+v", err)
			}
			continue
		} else {
			logger.NgapLog.Debugf("Set read buffer to %d bytes", readBufSize)
		}

		if err := newConn.SetReadTimeout(readTimeout); err != nil {
			logger.NgapLog.Errorf("Set read timeout error: %+v, accept failed", err)
			if err = newConn.Close(); err != nil {
				logger.NgapLog.Errorf("Close error: %+v", err)
			}
			continue
		} else {
			logger.NgapLog.Debugf("Set read timeout: %+v", readTimeout)
		}

		logger.NgapLog.Infof("[AMF] SCTP Accept from: %s", newConn.RemoteAddr().String())
		connections.Store(newConn, newConn)

		go handleUplinkConnection(newConn, readBufSize)
	}
}

func DialToAmf(addresses []string, port int) {
	var laddr *sctp.SCTPAddr
	sndbuf := 0
	rcvbuf := 0
	ips := []net.IPAddr{}

	for _, addr := range addresses {
		if netAddr, err := net.ResolveIPAddr("ip", addr); err != nil {
			logger.NgapLog.Errorf("Error resolving address '%s': %v\n", addr, err)
		} else {
			logger.NgapLog.Debugf("Resolved address '%s' to %s\n", addr, netAddr)
			ips = append(ips, *netAddr)
		}
	}

	addr := &sctp.SCTPAddr{
		IPAddrs: ips,
		Port:    port,
	}

	var lport int
	if lport != 0 {
		laddr = &sctp.SCTPAddr{
			Port: lport,
		}
	}

	amfConn, err = sctp.DialSCTP("sctp", laddr, addr)
	if err != nil {
		logger.NgapLog.Errorf("failed to dial: %v", err)
	}
	err = amfConn.SetWriteBuffer(sndbuf)
	if err != nil {
		logger.NgapLog.Errorf("failed to set write buf: %v", err)
	}
	err = amfConn.SetReadBuffer(rcvbuf)
	if err != nil {
		logger.NgapLog.Errorf("failed to set read buf: %v", err)
	}

	connections.Store(amfConn, amfConn)

	go handleDownlinkConnection(amfConn, readBufSize)
}

func handleUplinkConnection(conn *sctp.SCTPConn, bufsize uint32) {
	defer func() {
		// if AMF call Stop(), then conn.Close() will return EBADF because conn has been closed inside Stop()
		if err := conn.Close(); err != nil && err != syscall.EBADF {
			logger.NgapLog.Errorf("close connection error: %+v", err)
		}
		connections.Delete(conn)
	}()

	ppid := 0
	for {
		info := &sctp.SndRcvInfo{
			Stream: uint16(ppid),
			PPID:   uint32(ppid),
		}
		ppid += 1
		buf := make([]byte, bufsize)
		n, info, notification, err := conn.SCTPRead(buf)
		if err != nil {
			switch err {
			case io.EOF, io.ErrUnexpectedEOF:
				logger.NgapLog.Debugln("Read EOF from client")
				return
			case syscall.EAGAIN:
				logger.NgapLog.Debugln("SCTP read timeout")
				continue
			case syscall.EINTR:
				logger.NgapLog.Debugf("SCTPRead: %+v", err)
				continue
			default:
				logger.NgapLog.Errorf("Handle connection[addr: %+v] error: %+v", conn.RemoteAddr(), err)
				return
			}
		}

		if notification == nil {
			if info == nil || info.PPID != ngap.PPID {
				logger.NgapLog.Warnln("Received SCTP PPID != 60, discard this packet")
				continue
			}

			logger.NgapLog.Tracef("Read %d bytes", n)
			logger.NgapLog.Tracef("Packet content:\n%+v", hex.Dump(buf[:n]))

			// TODO: concurrent on per-UE message
			SendToAmf(amfConn, buf[:n], info)
			ppid += 1
		}
	}
}

func SendToAmf(conn *sctp.SCTPConn, msg []byte, info *sctp.SndRcvInfo) {
	bufsize := 256
	lbSelf := context.LB_Self()

	ran, ok := lbSelf.LbRanFindByConn(conn)
	if !ok {
		logger.NgapLog.Infof("Create a new NG connection for: %s", conn.RemoteAddr().String())
		ran = lbSelf.NewLbRan(conn)
	}

	if len(msg) == 0 {
		ran.Log.Infof("RAN close the connection.")
		ran.Remove()
		return
	}

	buf := make([]byte, bufsize)
	n, err := rand.Read(buf)
	if n != bufsize || err != nil {
		logger.NgapLog.Errorf("failed to generate random string len: %d", bufsize)
	}
	n, err = conn.SCTPWrite(msg, info)
	if err != nil {
		logger.NgapLog.Errorf("failed to write: %v", err)
	}
}

func handleDownlinkConnection(conn *sctp.SCTPConn, bufsize uint32) {
	defer func() {
		// if AMF call Stop(), then conn.Close() will return EBADF because conn has been closed inside Stop()
		if err := conn.Close(); err != nil && err != syscall.EBADF {
			logger.NgapLog.Errorf("close connection error: %+v", err)
		}
		connections.Delete(conn)
	}()
	ppid := 0
	for {
		info := &sctp.SndRcvInfo{
			Stream: uint16(ppid),
			PPID:   uint32(ppid),
		}
		ppid += 1
		amfConn.SubscribeEvents(sctp.SCTP_EVENT_DATA_IO)
		buf := make([]byte, bufsize)
		n, info, notification, err := conn.SCTPRead(buf)
		if err != nil {
			switch err {
			case io.EOF, io.ErrUnexpectedEOF:
				logger.NgapLog.Debugln("Read EOF from client")
				return
			case syscall.EAGAIN:
				logger.NgapLog.Debugln("SCTP read timeout")
				continue
			case syscall.EINTR:
				logger.NgapLog.Debugf("SCTPRead: %+v", err)
				continue
			default:
				logger.NgapLog.Errorf("Handle connection[addr: %+v] error: %+v", conn.RemoteAddr(), err)
				return
			}
		}

		if notification == nil {
			if info == nil || info.PPID != ngap.PPID {
				logger.NgapLog.Warnln("Received SCTP PPID != 60, discard this packet")
				continue
			}

			logger.NgapLog.Tracef("Read %d bytes", n)
			logger.NgapLog.Tracef("Packet content:\n%+v", hex.Dump(buf[:n]))
		}
		SendToRan(newConn, buf[:n], info)
	}
}

func SendToRan(conn *sctp.SCTPConn, msg []byte, info *sctp.SndRcvInfo) {
	var ran *context.LbRan
	bufsize := 256
	lbSelf := context.LB_Self()

	ran, ok := lbSelf.LbRanFindByConn(conn)
	if !ok {
		logger.NgapLog.Infof("Create a new NG connection for: %s", conn.RemoteAddr().String())
		ran = lbSelf.NewLbRan(conn)
	}

	if len(msg) == 0 {
		ran.Log.Infof("RAN close the connection.")
		ran.Remove()
		return
	}

	buf := make([]byte, bufsize)
	n, err := rand.Read(buf)
	if n != bufsize || err != nil {
		logger.NgapLog.Errorf("failed to generate random string len: %d", bufsize)
	}
	n, err = conn.SCTPWrite(msg, info)
	if err != nil {
		logger.NgapLog.Errorf("failed to write: %v", err)
	}
}

func Stop() {
	logger.NgapLog.Infof("Close SCTP server...")
	if err := sctpListener.Close(); err != nil {
		logger.NgapLog.Error(err)
		logger.NgapLog.Infof("SCTP server may not close normally.")
	}

	connections.Range(func(key, value interface{}) bool {
		conn := value.(net.Conn)
		if err := conn.Close(); err != nil {
			logger.NgapLog.Error(err)
		}
		return true
	})

	logger.NgapLog.Infof("SCTP server closed")
}
