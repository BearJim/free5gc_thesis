package ngap

import (
	"net"

	"git.cs.nctu.edu.tw/calee/sctp"

	"loadbalance/context"
	"loadbalance/logger"
)

func Dispatch(conn net.Conn, msg []byte) {
	var ran *context.LbRan
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

	var amf *context.LbAmf

	//set RAN configuration

	SendToRan(amf, msg)
}

func HandleSCTPNotification(conn net.Conn, notification sctp.Notification) {
	lbSelf := context.LB_Self()

	logger.NgapLog.Infof("Handle SCTP Notification[addr: %+v]", conn.RemoteAddr())

	ran, ok := lbSelf.LbRanFindByConn(conn)
	if !ok {
		logger.NgapLog.Warnf("RAN context has been removed[addr: %+v]", conn.RemoteAddr())
		return
	}

	switch notification.Type() {
	case sctp.SCTP_ASSOC_CHANGE:
		ran.Log.Infof("SCTP_ASSOC_CHANGE notification")
		event := notification.(*sctp.SCTPAssocChangeEvent)
		switch event.State() {
		case sctp.SCTP_COMM_LOST:
			ran.Log.Infof("SCTP state is SCTP_COMM_LOST, close the connection")
			ran.Remove()
		case sctp.SCTP_SHUTDOWN_COMP:
			ran.Log.Infof("SCTP state is SCTP_SHUTDOWN_COMP, close the connection")
			ran.Remove()
		default:
			ran.Log.Warnf("SCTP state[%+v] is not handled", event.State())
		}
	case sctp.SCTP_SHUTDOWN_EVENT:
		ran.Log.Infof("SCTP_SHUTDOWN_EVENT notification, close the connection")
		ran.Remove()
	default:
		ran.Log.Warnf("Non handled notification type: 0x%x", notification.Type())
	}
}

func SendToRan(amf *context.LbAmf, msg []byte) {
	if n, err := amf.Conn.Write(msg); err != nil {
		amf.Log.Errorf("Send error: %+v", err)
		return
	} else {
		amf.Log.Debugf("Write %d bytes", n)
	}
}
