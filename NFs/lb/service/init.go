package service

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"sync"
	"syscall"

	"github.com/sirupsen/logrus"
	"github.com/urfave/cli"

	mdaf "loadbalance/MDAF"
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

	if err := factory.CheckConfigVersion(); err != nil {
		return err
	}

	return nil
}

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
	// router.Use(cors.New(cors.Config{
	// 	AllowMethods: []string{"GET", "POST", "OPTIONS", "PUT", "PATCH", "DELETE"},
	// 	AllowHeaders: []string{
	// 		"Origin", "Content-Length", "Content-Type", "User-Agent", "Referrer", "Host",
	// 		"Token", "X-Requested-With",
	// 	},
	// 	ExposeHeaders:    []string{"Content-Length"},
	// 	AllowCredentials: true,
	// 	AllowAllOrigins:  true,
	// 	MaxAge:           86400,
	// }))

	initLog.Infoln("before add service")
	mdaf.AddService(router)

	initLog.Infoln("before ngap service")
	NgapIp := []string{"127.0.0.21"}
	AmfIp := []string{"127.0.0.18"}
	Amf1Ip := []string{"127.0.0.19"}
	Amf2Ip := []string{"127.0.0.20"}
	initLog.Infoln("DialToAmf")
	ngap_service.DialToAmf(AmfIp, 38412, 0)
	initLog.Infoln("DialToAmf1")
	ngap_service.DialToAmf(Amf1Ip, 38412, 1)
	initLog.Infoln("DialToAmf2")
	ngap_service.DialToAmf(Amf2Ip, 38412, 2)
	initLog.Infoln("Run ngap")
	ngap_service.Run(NgapIp, 38415)

	signalChannel := make(chan os.Signal, 1)
	signal.Notify(signalChannel, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-signalChannel
		lb.Terminate()
		os.Exit(0)
	}()

	initLog.Infoln("before http server")
	addr := fmt.Sprintf("%s:%d", "127.0.0.21", 8000)
	server, err := http2_util.NewServer(addr, path_util.Free5gcPath("free5gc/lbsslkey.log"), router)

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
