/*
 * Nsmf_PDUSession
 *
 * SMF PDU Session Service
 *
 * API version: 1.0.0
 * Generated by: OpenAPI Generator (https://openapi-generator.tech)
 */

package main

import (
	"fmt"
	"math/rand"
	"os"
	"time"

	"github.com/asaskevich/govalidator"
	"github.com/urfave/cli"

	"bitbucket.org/free5gc-team/smf/logger"
	"bitbucket.org/free5gc-team/smf/service"
	"bitbucket.org/free5gc-team/version"
)

var SMF = &service.SMF{}

func main() {
	app := cli.NewApp()
	app.Name = "smf"
	app.Usage = "5G Session Management Function (SMF)"
	app.Action = action
	app.Flags = SMF.GetCliCmd()
	rand.Seed(time.Now().UnixNano())

	if err := app.Run(os.Args); err != nil {
		logger.AppLog.Errorf("SMF Run error: %v\n", err)
	}
}

func action(c *cli.Context) error {
	if err := initLogFile(c.String("log"), c.String("log5gc")); err != nil {
		logger.AppLog.Errorf("%+v", err)
		return err
	}

	if err := SMF.Initialize(c); err != nil {
		switch errType := err.(type) {
		case govalidator.Errors:
			validErrs := err.(govalidator.Errors).Errors()
			for _, validErr := range validErrs {
				logger.CfgLog.Errorf("%+v", validErr)
			}
		default:
			logger.CfgLog.Errorf("%+v", errType)
		}
		logger.CfgLog.Errorf("[-- PLEASE REFER TO SAMPLE CONFIG FILE COMMENTS --]")
		return fmt.Errorf("Failed to initialize !!")
	}

	logger.AppLog.Infoln(c.App.Name)
	logger.AppLog.Infoln("SMF version: ", version.GetVersion())

	SMF.Start()

	return nil
}

func initLogFile(logNfPath, log5gcPath string) error {
	if err := logger.LogFileHook(logNfPath, log5gcPath); err != nil {
		return err
	}
	return nil
}
