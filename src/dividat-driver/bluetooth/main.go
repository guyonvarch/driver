package bluetooth

/* Monitor bluetooth devices.

Maintain a list of all the connected bluetooth devices.

*/

import (
	"fmt"

	"github.com/godbus/dbus/v5"
	"github.com/muka/go-bluetooth/api"
	"github.com/muka/go-bluetooth/bluez/profile/adapter"
	"github.com/muka/go-bluetooth/bluez/profile/agent"
	"github.com/muka/go-bluetooth/bluez/profile/device"
	log "github.com/sirupsen/logrus"
)

const passCode = "1234"

var connectedDevices = map[string]*device.Device1{}

func GetConnectedDevices() map[string]*device.Device1 {
	return connectedDevices
}

func Monitor() {
	defer api.Exit()

	dbusConn, err := dbus.SystemBus()
	if err != nil {
		log.WithError(err).Panic("Error connecting to dbus.")
		return
	}

	simpleAgent := agent.NewSimpleAgent()
	simpleAgent.SetPassCode(passCode)
	err = agent.ExposeAgent(dbusConn, simpleAgent, agent.CapKeyboardDisplay, true)
	if err != nil {
		log.WithError(err).Panic("Error exposing Bluetooth agent.")
		return
	}

	adap, err := adapter.GetDefaultAdapter()
	if err != nil {
		log.WithError(err).Panic("Error getting Bluetooth default adapter.")
		return
	}

	err = adap.FlushDevices()
	if err != nil {
		log.WithError(err).Panic("Error flushing Bluetooth devices.")
		return
	}

	log.Info("Starting Bluetooth monitoring.")
	discovery, cancel, err := api.Discover(adap, nil)
	if err != nil {
		log.WithError(err).Panic("Error discovering Bluetooth devices.")
		return
	}
	defer cancel()

	for ev := range discovery {
		if ev.Type == adapter.DeviceRemoved {
			log.Infof("Bluetooth device removed: %s", ev.Path)
			delete(connectedDevices, fmt.Sprintf("%s", ev.Path))
			continue
		}

		dev, err := device.NewDevice1(ev.Path)
		if err != nil {
			log.WithError(err).Errorf("Error getting Bluetooth device from path: %s", ev.Path)
			continue
		}

		if dev == nil {
			log.Errorf("Bluetooth device not found from path: %s", ev.Path)
			continue
		}

		log.Infof("New Bluetooth device: %s %s", dev.Properties.Name, dev.Properties.Address)
		connectedDevices[fmt.Sprintf("%s", ev.Path)] = dev
	}
}

func Pair(devPath string, dev *device.Device1) error {
	err := dev.Pair()
	if err != nil {
		return err
	}
	dev.Properties.Paired = true
	connectedDevices[devPath] = dev
	return nil
}

func FlushDevices() {
	adap, err := adapter.GetDefaultAdapter()
	if err != nil {
		log.WithError(err).Error("Error getting Bluetooth default adapter.")
		return
	}

	err = adap.FlushDevices()
	if err != nil {
		log.WithError(err).Error("Error flushing Bluetooth devices.")
		return
	}
}
