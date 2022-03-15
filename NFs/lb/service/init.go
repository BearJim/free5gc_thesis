package service

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"sync"
	"syscall"

	"github.com/gin-contrib/cors"
	"github.com/sirupsen/logrus"
	"github.com/urfave/cli"

	"loadbalance/factory"

	"loadbalance/logger"
	ngap_service "loadbalance/ngap/service"

	"github.com/free5gc/http2_util"
	"github.com/free5gc/logger_util"
	"github.com/free5gc/path_util"
)

type LB struct{}

type (
	// Config information.
	Config struct {
		lbcfg string
	}
)

var config Config

var lbCLi = []cli.Flag{
	cli.StringFlag{
		Name:  "free5gccfg",
		Usage: "common config file",
	},
	cli.StringFlag{
		Name:  "lbcfg",
		Usage: "lb config file",
	},
}

var initLog *logrus.Entry

func init() {
	initLog = logger.InitLog
}

func (*LB) GetCliCmd() (flags []cli.Flag) {
	return lbCLi
}

func (lb *LB) Initialize(c *cli.Context) error {
	config = Config{
		lbcfg: c.String("lbcfg"),
	}

	if config.lbcfg != "" {
		if err := factory.InitConfigFactory(config.lbcfg); err != nil {
			return err
		}
	} else {
		DefaultLbConfigPath := path_util.Free5gcPath("free5gc/config/lbcfg.yaml")
		if err := factory.InitConfigFactory(DefaultLbConfigPath); err != nil {
			return err
		}
	}

	// lb.setLogLevel()

	if err := factory.CheckConfigVersion(); err != nil {
		return err
	}

	return nil
}

// func (lb *LB) setLogLevel() {
// 	if factory.LbConfig.Logger == nil {
// 		initLog.Warnln("LB config without log level setting!!!")
// 		return
// 	}

// 	if factory.LbConfig.Logger.LB != nil {
// 		if factory.LbConfig.Logger.LB.DebugLevel != "" {
// 			if level, err := logrus.ParseLevel(factory.LbConfig.Logger.LB.DebugLevel); err != nil {
// 				initLog.Warnf("LB Log level [%s] is invalid, set to [info] level",
// 					factory.LbConfig.Logger.LB.DebugLevel)
// 				logger.SetLogLevel(logrus.InfoLevel)
// 			} else {
// 				initLog.Infof("LB Log level is set to [%s] level", level)
// 				logger.SetLogLevel(level)
// 			}
// 		} else {
// 			initLog.Warnln("LB Log level not set. Default set to [info] level")
// 			logger.SetLogLevel(logrus.InfoLevel)
// 		}
// 		logger.SetReportCaller(factory.LbConfig.Logger.LB.ReportCaller)
// 	}

// 	if factory.LbConfig.Logger.NAS != nil {
// 		if factory.LbConfig.Logger.NAS.DebugLevel != "" {
// 			if level, err := logrus.ParseLevel(factory.LbConfig.Logger.NAS.DebugLevel); err != nil {
// 				nasLogger.NasLog.Warnf("NAS Log level [%s] is invalid, set to [info] level",
// 					factory.LbConfig.Logger.NAS.DebugLevel)
// 				logger.SetLogLevel(logrus.InfoLevel)
// 			} else {
// 				nasLogger.SetLogLevel(level)
// 			}
// 		} else {
// 			nasLogger.NasLog.Warnln("NAS Log level not set. Default set to [info] level")
// 			nasLogger.SetLogLevel(logrus.InfoLevel)
// 		}
// 		nasLogger.SetReportCaller(factory.LbConfig.Logger.NAS.ReportCaller)
// 	}

// 	if factory.LbConfig.Logger.NGAP != nil {
// 		if factory.LbConfig.Logger.NGAP.DebugLevel != "" {
// 			if level, err := logrus.ParseLevel(factory.LbConfig.Logger.NGAP.DebugLevel); err != nil {
// 				ngapLogger.NgapLog.Warnf("NGAP Log level [%s] is invalid, set to [info] level",
// 					factory.LbConfig.Logger.NGAP.DebugLevel)
// 				ngapLogger.SetLogLevel(logrus.InfoLevel)
// 			} else {
// 				ngapLogger.SetLogLevel(level)
// 			}
// 		} else {
// 			ngapLogger.NgapLog.Warnln("NGAP Log level not set. Default set to [info] level")
// 			ngapLogger.SetLogLevel(logrus.InfoLevel)
// 		}
// 		ngapLogger.SetReportCaller(factory.LbConfig.Logger.NGAP.ReportCaller)
// 	}

// 	if factory.LbConfig.Logger.FSM != nil {
// 		if factory.LbConfig.Logger.FSM.DebugLevel != "" {
// 			if level, err := logrus.ParseLevel(factory.LbConfig.Logger.FSM.DebugLevel); err != nil {
// 				fsmLogger.FsmLog.Warnf("FSM Log level [%s] is invalid, set to [info] level",
// 					factory.LbConfig.Logger.FSM.DebugLevel)
// 				fsmLogger.SetLogLevel(logrus.InfoLevel)
// 			} else {
// 				fsmLogger.SetLogLevel(level)
// 			}
// 		} else {
// 			fsmLogger.FsmLog.Warnln("FSM Log level not set. Default set to [info] level")
// 			fsmLogger.SetLogLevel(logrus.InfoLevel)
// 		}
// 		fsmLogger.SetReportCaller(factory.LbConfig.Logger.FSM.ReportCaller)
// 	}

// 	if factory.LbConfig.Logger.Aper != nil {
// 		if factory.LbConfig.Logger.Aper.DebugLevel != "" {
// 			if level, err := logrus.ParseLevel(factory.LbConfig.Logger.Aper.DebugLevel); err != nil {
// 				aperLogger.AperLog.Warnf("Aper Log level [%s] is invalid, set to [info] level",
// 					factory.LbConfig.Logger.Aper.DebugLevel)
// 				aperLogger.SetLogLevel(logrus.InfoLevel)
// 			} else {
// 				aperLogger.SetLogLevel(level)
// 			}
// 		} else {
// 			aperLogger.AperLog.Warnln("Aper Log level not set. Default set to [info] level")
// 			aperLogger.SetLogLevel(logrus.InfoLevel)
// 		}
// 		aperLogger.SetReportCaller(factory.LbConfig.Logger.Aper.ReportCaller)
// 	}

// 	if factory.LbConfig.Logger.PathUtil != nil {
// 		if factory.LbConfig.Logger.PathUtil.DebugLevel != "" {
// 			if level, err := logrus.ParseLevel(factory.LbConfig.Logger.PathUtil.DebugLevel); err != nil {
// 				pathUtilLogger.PathLog.Warnf("PathUtil Log level [%s] is invalid, set to [info] level",
// 					factory.LbConfig.Logger.PathUtil.DebugLevel)
// 				pathUtilLogger.SetLogLevel(logrus.InfoLevel)
// 			} else {
// 				pathUtilLogger.SetLogLevel(level)
// 			}
// 		} else {
// 			pathUtilLogger.PathLog.Warnln("PathUtil Log level not set. Default set to [info] level")
// 			pathUtilLogger.SetLogLevel(logrus.InfoLevel)
// 		}
// 		pathUtilLogger.SetReportCaller(factory.LbConfig.Logger.PathUtil.ReportCaller)
// 	}

// 	if factory.LbConfig.Logger.OpenApi != nil {
// 		if factory.LbConfig.Logger.OpenApi.DebugLevel != "" {
// 			if level, err := logrus.ParseLevel(factory.LbConfig.Logger.OpenApi.DebugLevel); err != nil {
// 				openApiLogger.OpenApiLog.Warnf("OpenAPI Log level [%s] is invalid, set to [info] level",
// 					factory.LbConfig.Logger.OpenApi.DebugLevel)
// 				openApiLogger.SetLogLevel(logrus.InfoLevel)
// 			} else {
// 				openApiLogger.SetLogLevel(level)
// 			}
// 		} else {
// 			openApiLogger.OpenApiLog.Warnln("OpenAPI Log level not set. Default set to [info] level")
// 			openApiLogger.SetLogLevel(logrus.InfoLevel)
// 		}
// 		openApiLogger.SetReportCaller(factory.LbConfig.Logger.OpenApi.ReportCaller)
// 	}
// }

func (lb *LB) FilterCli(c *cli.Context) (args []string) {
	for _, flag := range lb.GetCliCmd() {
		name := flag.GetName()
		value := fmt.Sprint(c.Generic(name))
		if value == "" {
			continue
		}

		args = append(args, "--"+name, value)
	}
	return args
}

func (lb *LB) Start() {
	initLog.Infoln("Server started")

	router := logger_util.NewGinWithLogrus(logger.GinLog)
	router.Use(cors.New(cors.Config{
		AllowMethods: []string{"GET", "POST", "OPTIONS", "PUT", "PATCH", "DELETE"},
		AllowHeaders: []string{
			"Origin", "Content-Length", "Content-Type", "User-Agent", "Referrer", "Host",
			"Token", "X-Requested-With",
		},
		ExposeHeaders:    []string{"Content-Length"},
		AllowCredentials: true,
		AllowAllOrigins:  true,
		MaxAge:           86400,
	}))

	// httpcallback.AddService(router)
	// oam.AddService(router)
	// for _, serviceName := range factory.LbConfig.Configuration.ServiceNameList {
	// 	switch models.ServiceName(serviceName) {
	// 	case models.ServiceName_NAMF_COMM:
	// 		communication.AddService(router)
	// 	case models.ServiceName_NAMF_EVTS:
	// 		eventexposure.AddService(router)
	// 	case models.ServiceName_NAMF_MT:
	// 		mt.AddService(router)
	// 	case models.ServiceName_NAMF_LOC:
	// 		location.AddService(router)
	// 	}
	// }

	// self := context.LB_Self()
	// util.InitAmfContext(self)

	// addr := fmt.Sprintf("%s:%d", self.BindingIPv4, self.SBIPort)

	// ngapHandler := ngap_service.NGAPHandler{
	// 	HandleMessage:      ngap.Dispatch,
	// 	HandleNotification: ngap.HandleSCTPNotification,
	// }

	var NgapIp []string
	var AmfIp []string
	NgapIp[0] = "127.0.0.21"
	AmfIp[0] = "127.0.0.18"
	// AmfIp[1] = "127.0.0.19"
	// AmfIp[2] = "127.0.0.20"
	ngap_service.DialToAmf(AmfIp, 38412)
	ngap_service.Run(NgapIp, 38415)

	// Register to NRF
	// var profile models.NfProfile
	// if profileTmp, err := consumer.BuildNFInstance(self); err != nil {
	// 	initLog.Error("Build LB Profile Error")
	// } else {
	// 	profile = profileTmp
	// }

	// if _, nfId, err := consumer.SendRegisterNFInstance(self.NrfUri, self.NfId, profile); err != nil {
	// 	initLog.Warnf("Send Register NF Instance failed: %+v", err)
	// } else {
	// 	self.NfId = nfId
	// }

	signalChannel := make(chan os.Signal, 1)
	signal.Notify(signalChannel, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-signalChannel
		lb.Terminate()
		os.Exit(0)
	}()

	addr := fmt.Sprintf("%s:%d", "127.0.0.21", 8000)
	LbLogPath := "free5gc/ausfsslkey.log"
	server, err := http2_util.NewServer(addr, LbLogPath, router)

	if server == nil {
		initLog.Errorf("Initialize HTTP server failed: %+v", err)
		return
	}

	if err != nil {
		initLog.Warnf("Initialize HTTP server: %+v", err)
	}

	serverScheme := "http"
	if serverScheme == "http" {
		err = server.ListenAndServe()
	}

	if err != nil {
		initLog.Fatalf("HTTP server setup failed: %+v", err)
	}
}

func (lb *LB) Exec(c *cli.Context) error {
	// LB.Initialize(cfgPath, c)

	initLog.Traceln("args:", c.String("lbcfg"))
	args := lb.FilterCli(c)
	initLog.Traceln("filter: ", args)
	command := exec.Command("./lb", args...)

	stdout, err := command.StdoutPipe()
	if err != nil {
		initLog.Fatalln(err)
	}
	wg := sync.WaitGroup{}
	wg.Add(3)
	go func() {
		in := bufio.NewScanner(stdout)
		for in.Scan() {
			fmt.Println(in.Text())
		}
		wg.Done()
	}()

	stderr, err := command.StderrPipe()
	if err != nil {
		initLog.Fatalln(err)
	}
	go func() {
		in := bufio.NewScanner(stderr)
		for in.Scan() {
			fmt.Println(in.Text())
		}
		wg.Done()
	}()

	go func() {
		if err = command.Start(); err != nil {
			initLog.Errorf("LB Start error: %+v", err)
		}
		wg.Done()
	}()

	wg.Wait()

	return err
}

// Used in LB planned removal procedure
func (lb *LB) Terminate() {
	logger.InitLog.Infof("Terminating LB...")
	// lbSelf := context.LB_Self()

	// // TODO: forward registered UE contexts to target LB in the same LB set if there is one

	// // deregister with NRF
	// problemDetails, err := consumer.SendDeregisterNFInstance()
	// if problemDetails != nil {
	// 	logger.InitLog.Errorf("Deregister NF instance Failed Problem[%+v]", problemDetails)
	// } else if err != nil {
	// 	logger.InitLog.Errorf("Deregister NF instance Error[%+v]", err)
	// } else {
	// 	logger.InitLog.Infof("[LB] Deregister from NRF successfully")
	// }

	// // send LB status indication to ran to notify ran that this LB will be unavailable
	// logger.InitLog.Infof("Send LB Status Indication to Notify RANs due to LB terminating")
	// unavailableGuamiList := ngap_message.BuildUnavailableGUAMIList(lbSelf.ServedGuamiList)
	// lbSelf.AmfRanPool.Range(func(key, value interface{}) bool {
	// 	ran := value.(*context.AmfRan)
	// 	ngap_message.SendAMFStatusIndication(ran, unavailableGuamiList)
	// 	return true
	// })

	ngap_service.Stop()

	// callback.SendAmfStatusChangeNotify((string)(models.StatusChange_UNAVAILABLE), lbSelf.ServedGuamiList)
	logger.InitLog.Infof("LB terminated")
}
