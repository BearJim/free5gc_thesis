package main

import (
	"fmt"
	"os"

	"github.com/sirupsen/logrus"
	"github.com/urfave/cli"

	"lb/logger"
	"lb/service"

	"github.com/free5gc/version"
)

var LB = &service.LB{}

var appLog *logrus.Entry

func init() {
	appLog = logger.AppLog
}

func main() {
	app := cli.NewApp()
	app.Name = "lb"
	appLog.Infoln(app.Name)
	appLog.Infoln("LB version: ", version.GetVersion())
	app.Usage = "-free5gccfg common configuration file -lbcfg lb configuration file"
	app.Action = action
	app.Flags = LB.GetCliCmd()
	if err := app.Run(os.Args); err != nil {
		appLog.Errorf("LB Run error: %v", err)
		return
	}
}

func action(c *cli.Context) error {
	if err := LB.Initialize(c); err != nil {
		logger.CfgLog.Errorf("%+v", err)
		return fmt.Errorf("Failed to initialize !!")
	}

	LB.Start()

	return nil
}
