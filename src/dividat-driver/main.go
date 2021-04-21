package main

// Start up driver as a service

import (
	"context"
	"io/ioutil"
	"log"
	"os"

	"github.com/dividat/driver/src/dividat-driver/bluetooth"
	"github.com/dividat/driver/src/dividat-driver/firmware"
	"github.com/dividat/driver/src/dividat-driver/logging"
	"github.com/dividat/driver/src/dividat-driver/server"
	"github.com/kardianos/service"
	"github.com/sirupsen/logrus"
)

type program struct {
	close context.CancelFunc
}

func main() {
	// Serve command or start in daemon mode by default
	if len(os.Args) > 1 && os.Args[1] == "update-firmware" {
		firmware.Command(os.Args[2:])
	} else {
		runDaemon()
	}
}

func (p *program) Start(s service.Service) error {
	// Set up logging
	logger := logrus.New()
	if !service.Interactive() {
		logger.Out = ioutil.Discard
		if systemLogger, err := s.SystemLogger(nil); err == nil {
			logger.AddHook(logging.NewSystemHook(systemLogger))
		}
	}
	logger.SetLevel(logrus.DebugLevel)

	p.close = server.Start(logger)
	return nil
}

func (p *program) Stop(s service.Service) error {
	p.close()
	return nil
}

func runDaemon() {
	svcConfig := &service.Config{
		Name:        "DividatDriver",
		DisplayName: "Dividat Driver",
		Description: "Dividat Driver application for hardware connectivity.",
	}

	prg := &program{}
	s, err := service.New(prg, svcConfig)
	if err != nil {
		log.Fatal(err)
	}

	go bluetooth.Monitor()

	log.Fatal(s.Run())
}
