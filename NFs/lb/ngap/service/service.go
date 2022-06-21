package service

import (
	"crypto/rand"
	"io"
	"net"
	"sync"
	"syscall"

	"git.cs.nctu.edu.tw/calee/sctp"

	"loadbalance/context"
	"loadbalance/logger"

	"github.com/free5gc/ngap"
	"github.com/free5gc/ngap/ngapType"
)

type NGAPHandler struct {
	HandleMessage      func(conn net.Conn, msg []byte)
	HandleNotification func(conn net.Conn, notification sctp.Notification)
}

const readBufSize uint32 = 8192

// set default read timeout to 2 seconds
var readTimeout syscall.Timeval = syscall.Timeval{Sec: 2, Usec: 0}

var (
	sctpListener    *sctp.SCTPListener
	connections     sync.Map
	amfConn         *sctp.SCTPConn
	amfConn0        *sctp.SCTPConn
	amfConn1        *sctp.SCTPConn
	amfConn2        *sctp.SCTPConn
	newConn         *sctp.SCTPConn
	goAmf           int   //實際要去哪個AMF
	oldGoAmf        []int //舊策略
	nowGoAmf        []int //正在執行中的策略，用來計數
	countSum        int   //用來計算分配一論的次數，為oldGoAmf的總和
	count           int   //用來計當前輪到哪個AMF0~2, count = count%3 < countSum
	err             error
	NGsetupResponse bool
	// initailMsgCount int //用initailMsgCount做暫時RR
)

var sctpConfig sctp.SocketConfig = sctp.SocketConfig{
	InitMsg:   sctp.InitMsg{NumOstreams: 3, MaxInstreams: 5, MaxAttempts: 2, MaxInitTimeout: 2},
	RtoInfo:   &sctp.RtoInfo{SrtoAssocID: 0, SrtoInitial: 500, SrtoMax: 1500, StroMin: 100},
	AssocInfo: &sctp.AssocInfo{AsocMaxRxt: 4},
}

// var mUEAMF map[*ngapType.RANUENGAPID]int

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
	NGsetupResponse = false
	// initailMsgCount = 0
	go listenAndServe(addr)
}

func listenAndServe(addr *sctp.SCTPAddr) {
	if listener, err := sctpConfig.Listen("sctp", addr); err != nil {
		logger.NgapLog.Errorf("Failed to listen: %+v", err)
		return
	} else {
		sctpListener = listener
	}

	logger.NgapLog.Infof("71 Listen on %s", sctpListener.Addr())

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
			logger.NgapLog.Printf("Set read buffer to %d bytes", readBufSize)
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

func DialToAmf(addresses []string, port int, nAmf int) {
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

	logger.NgapLog.Infof("raw addr: %+v\n", addr.ToRawSockAddrBuf())

	lport := 0
	if lport != 0 {
		laddr = &sctp.SCTPAddr{
			Port: lport,
		}
	}

	amfConn, err = sctp.DialSCTP("sctp", laddr, addr)
	if err != nil {
		logger.NgapLog.Errorf("failed to dial 177: %v", err)
	}

	switch nAmf {
	case 0:
		amfConn0 = amfConn
		connections.Store(amfConn0, amfConn0)
		logger.NgapLog.Infof("this is amf 0")
	case 1:
		amfConn1 = amfConn
		connections.Store(amfConn1, amfConn1)
		logger.NgapLog.Infof("this is amf 1")
	case 2:
		amfConn2 = amfConn
		connections.Store(amfConn2, amfConn2)
		logger.NgapLog.Infof("this is amf 2")
	}

	if amfConn != nil {
		logger.NgapLog.Infof("Dail LocalAddr: %s; RemoteAddr: %s", amfConn.LocalAddr(), amfConn.RemoteAddr())

		err = amfConn.SetWriteBuffer(sndbuf)
		if err != nil {
			logger.NgapLog.Errorf("failed to set write buf: %v", err)
		}
		err = amfConn.SetReadBuffer(rcvbuf)
		if err != nil {
			logger.NgapLog.Errorf("failed to set read buf: %v", err)
		}
		go handleDownlinkConnection(amfConn, readBufSize)
	}
}

func handleUplinkConnection(conn *sctp.SCTPConn, bufsize uint32) {
	defer func() {
		// if LB call Stop(), then conn.Close() will return EBADF because conn has been closed inside Stop()
		if err := conn.Close(); err != nil && err != syscall.EBADF {
			logger.NgapLog.Errorf("close connection error: %+v", err)
		}
		connections.Delete(conn)
	}()

	mUEAMF := make(map[ngapType.RANUENGAPID]int)
	ppid := 0
	for {
		info := &sctp.SndRcvInfo{
			Stream: uint16(ppid),
			PPID:   uint32(ppid),
		}
		ppid += 1
		bufUp := make([]byte, bufsize)
		n, info, notification, err := conn.SCTPRead(bufUp)
		if err != nil {
			switch err {
			case io.EOF, io.ErrUnexpectedEOF:
				logger.NgapLog.Debugf("Read EOF from client")
				return
			case syscall.EAGAIN:
				logger.NgapLog.Debugf("SCTP read timeout")
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

			msg := bufUp[:n]
			pdu, err := ngap.Decoder(msg)
			if err != nil {
				logger.NgapLog.Errorf("NGAP decode error : %+v", err)
				return
			}

			// decode nas for NGSETUP
			switch pdu.Present {
			case ngapType.NGAPPDUPresentInitiatingMessage:
				initiatingMessage := pdu.InitiatingMessage
				switch initiatingMessage.ProcedureCode.Value {
				case ngapType.ProcedureCodeNGSetup: //NGSETUP
					goAmf = 3
					logger.NgapLog.Infof("NGSETUP, send to AMF0 AMF1 AMF2, goAmf: ", goAmf)
				case ngapType.ProcedureCodeInitialUEMessage: // initail UE message
					// logger.NgapLog.Infof("initail UE message: ", initailMsgCount) //用initailMsgCount做暫時RR
					// MDAF need to decide which AMF to go
					newGoAmf := context.LB_Self().MdafGoAmf //抓MDAF的新策略
					checkChange(newGoAmf)                   //確認是否更新策略，有更新:重新計數
					goAmf = getGoAMf()
					for _, ie := range initiatingMessage.Value.InitialUEMessage.ProtocolIEs.List {
						switch ie.Id.Value {
						case ngapType.ProtocolIEIDRANUENGAPID:
							rANUENGAPID := ie.Value.RANUENGAPID
							value, isExist := mUEAMF[*rANUENGAPID]
							if !isExist {
								logger.NgapLog.Infof("is not Exist")
								// mUEAMF[*rANUENGAPID] = goAmf + initailMsgCount
								mUEAMF[*rANUENGAPID] = goAmf
								goAmf = mUEAMF[*rANUENGAPID]
							} else {
								logger.NgapLog.Infof("is Exist")
								goAmf = value
							}
							logger.NgapLog.Errorf("mUEAMF: ", mUEAMF)
							logger.NgapLog.Errorf("mUEAMF[rANUENGAPID]: %d", mUEAMF[*rANUENGAPID])
							logger.NgapLog.Infof("mUEAMF key: %v , goAmf: %d", rANUENGAPID, goAmf)
							logger.NgapLog.Trace("Decode IE RANUENGAPID")
							if rANUENGAPID == nil {
								logger.NgapLog.Error("RANUENGAPID is nil")
								return
							}
						}
					}
					// if initailMsgCount >= 2 {
					// 	initailMsgCount = 0
					// } else {
					// 	initailMsgCount += 1
					// }
				case ngapType.ProcedureCodeUplinkNASTransport: // Uplink NAS Transport
					for i := 0; i < len(initiatingMessage.Value.UplinkNASTransport.ProtocolIEs.List); i++ {
						ie := initiatingMessage.Value.UplinkNASTransport.ProtocolIEs.List[i]
						switch ie.Id.Value {
						case ngapType.ProtocolIEIDRANUENGAPID:
							rANUENGAPID := ie.Value.RANUENGAPID
							value, isExist := mUEAMF[*rANUENGAPID]
							logger.NgapLog.Infof("Uplink NAS Transport is Exist?: %v", isExist)
							goAmf = value

							logger.NgapLog.Infof("mUEAMF: ", mUEAMF)
							logger.NgapLog.Infof("mUEAMF key: %v , goAmf: %d", rANUENGAPID, goAmf)
							logger.NgapLog.Trace("Decode IE RANUENGAPID")
							if rANUENGAPID == nil {
								logger.NgapLog.Error("RANUENGAPID is nil")
								return
							}
						}
					}
				}
			case ngapType.NGAPPDUPresentSuccessfulOutcome:
				successfulOutcome := pdu.SuccessfulOutcome
				switch successfulOutcome.ProcedureCode.Value {
				case ngapType.ProcedureCodeInitialContextSetup:
					initialContextSetupResponse := successfulOutcome.Value.InitialContextSetupResponse
					for _, ie := range initialContextSetupResponse.ProtocolIEs.List {
						switch ie.Id.Value {
						case ngapType.ProtocolIEIDRANUENGAPID:
							rANUENGAPID := ie.Value.RANUENGAPID
							value := mUEAMF[*rANUENGAPID]
							goAmf = value
							logger.NgapLog.Infof("mUEAMF key: %d , goAmf: %d", rANUENGAPID, value)
							logger.NgapLog.Trace("Decode IE RANUENGAPID")
							logger.NgapLog.Infof("RANUENGAPID: %d", rANUENGAPID)
							if rANUENGAPID == nil {
								logger.NgapLog.Error("RANUENGAPID is nil")
								return
							}
						}
					}
				}
			}

			switch goAmf {
			case 0:
				logger.NgapLog.Info("Send to AMf number: ", goAmf)
				SendToAmf(amfConn0, bufUp[:n], info)
			case 1:
				logger.NgapLog.Info("Send to AMf number: ", goAmf)
				SendToAmf(amfConn1, bufUp[:n], info)
			case 2:
				logger.NgapLog.Info("Send to AMf number: ", goAmf)
				SendToAmf(amfConn2, bufUp[:n], info)
			case 3:
				logger.NgapLog.Info("NGSetup send to all AMfs")
				SendToAmf(amfConn0, bufUp[:n], info)
				SendToAmf(amfConn1, bufUp[:n], info)
				SendToAmf(amfConn2, bufUp[:n], info)
			default:
				logger.NgapLog.Info("Send to AMf number: Default, goAmf = ", goAmf)
				SendToAmf(amfConn, bufUp[:n], info)
			}
		}
	}
}

func SendToAmf(conn *sctp.SCTPConn, msg []byte, info *sctp.SndRcvInfo) {
	bufsize := 256
	lbSelf := context.LB_Self()

	ran, ok := lbSelf.LbRanFindByConn(conn)
	logger.NgapLog.Info("SendToAmf")
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
		logger.NgapLog.Errorf("failed to write to AMF: %v", err)
	} else {
		logger.NgapLog.Infof("write to amf: len %d", n)
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
		buf := make([]byte, bufsize)
		logger.NgapLog.Info("Downlink before Read")
		n, info, notification, err := conn.SCTPRead(buf)
		logger.NgapLog.Info("Downlink after Read")
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
				logger.NgapLog.Tracef("Received SCTP PPID != 60")
				// continue
			}

			msg := buf[:n]
			pdu, err := ngap.Decoder(msg)
			if err != nil {
				logger.NgapLog.Errorf("NGAP decode error : %+v", err)
				return
			}
			switch pdu.Present {
			case ngapType.NGAPPDUPresentSuccessfulOutcome:
				successfulOutcome := pdu.SuccessfulOutcome
				switch successfulOutcome.ProcedureCode.Value {
				case ngapType.ProcedureCodeNGSetup:
					if !NGsetupResponse { //NGsetupResponse = false, send 1 response to RAN
						NGsetupResponse = true
						if newConn != nil {
							SendToRan(newConn, buf[:n], info)
						}
					} else { //NGsetupResponse = ture, drop other response
						logger.NgapLog.Infof("NGSETUP response drop")
					}
				}
			default:
				if newConn != nil {
					SendToRan(newConn, buf[:n], info)
				}
			}
		}
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
		logger.NgapLog.Errorf("failed to write to RAN: %v", err)
	} else {
		logger.NgapLog.Infof("write: len %d", n)
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

func checkChange(newGoAmf []int) {
	change := sliceEqual(newGoAmf, oldGoAmf) //是否更新策略
	if change {
		oldGoAmf = newGoAmf //更新策略
		nowGoAmf = oldGoAmf
		for _, v := range oldGoAmf {
			countSum += v //計算新countSum
		}
		count = 0 //如果更新策略，重新一輪
	}
}

func sliceEqual(a, b []int) bool {
	if len(a) != len(b) {
		return true
	}

	if (a == nil) != (b == nil) {
		return true
	}

	for i, v := range a {
		if v != b[i] {
			return true
		}
	}

	return false //表示兩者相同，沒有更新策略
}

func getGoAMf() (amfNum int) {
	if count >= countSum { //如果超過一輪次數，重新一輪
		count = 0
		nowGoAmf = oldGoAmf
	}
	if nowGoAmf[count%3] != 0 {
		nowGoAmf[count%3] -= 1
		amfNum = count % 3
		count++
	} else {
		count++
		amfNum = getGoAMf()
	}
	return
}

// func getGoAMf(l []int) (amfNum int) { //回傳slice中的位址 代表哪個AMF
// 	typeCount := 0     //用以辨別type 0和3
// 	tempMaxAmf := 4    //現在要給最多負載的AMF
// 	tempGoAmfType := 4 //屬於哪種分派
// 	for key, v := range l {
// 		if v == 4 {
// 			tempGoAmfType = 1
// 			tempMaxAmf = key
// 			break
// 		} else if v == 3 {
// 			tempGoAmfType = 2
// 			tempMaxAmf = key
// 			break
// 		} else if v == 1 {
// 			typeCount += 1
// 			if typeCount == 3 {
// 				tempGoAmfType = 3
// 				break
// 			} else if typeCount == 1 {
// 				tempGoAmfType = 0
// 			}
// 		}
// 	}
// 	if tempGoAmfType == goAmfType {
// 		if tempMaxAmf == nowMaxAmf { //type 0 or 1 or 2 沒改變
// 			switch goAmfType {
// 			case 0: // 1 0 0
// 				amfNum = nowMaxAmf
// 			case 1: // 1 4 0
// 				if amfCount > 1 {
// 					amfCount -= 1
// 					amfNum = nowMaxAmf
// 				} else {
// 					amfCount = 5
// 					for key, v := range l {
// 						if v == 1 {
// 							amfNum = key
// 							break
// 						}
// 					}
// 				}
// 			case 2: // 1 1 3
// 				if amfCount > 2 {
// 					amfCount -= 1
// 					amfNum = nowMaxAmf
// 				} else {
// 					for key, v := range l {
// 						if v == 1 {
// 							if !pickAmf {
// 								amfNum = key
// 								pickAmf = true
// 								break
// 							} else {
// 								pickAmf = false
// 								amfCount = 5
// 								continue //跳過一次看到數字1
// 							}
// 						}
// 					}
// 				}
// 			}
// 		} else if goAmfType == 3 { // type 3 round robin
// 			amfNum = nowMaxAmf
// 			nowMaxAmf += 1
// 			if nowMaxAmf > 2 {
// 				nowMaxAmf = 0
// 			}
// 		} else { //決策變更但同type，不會是type 3 ex. 140->401 , 113->311
// 			nowMaxAmf = tempMaxAmf
// 			amfNum = nowMaxAmf
// 			amfCount = 4 //這次也算一次，5-1=4
// 		}

// 	} else { //決策變更且type變更
// 		nowMaxAmf = tempMaxAmf
// 		goAmfType = tempGoAmfType
// 		amfNum = nowMaxAmf
// 		amfCount = 4 //這次也算一次，5-1=4
// 	}

// 	return
// }
