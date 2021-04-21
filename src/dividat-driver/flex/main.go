package flex

/* Connects to Senso Flex devices through a serial connection or Bluetooth and
combines serial data into measurement sets.

This helps establish an indirect WebSocket connection to receive a stream of
samples from the device.

The functionality of this module is as follows:

- While connected, scan for serial devices or Bluetooth devices that look like
a potential Flex device.
- Connect to suitable devices and start polling for measurements.
- Minimally parse incoming data to determine start and end of a measurement.
- Send each complete measurement set to client as a binary package.

*/

import (
	"bufio"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/cskr/pubsub"
	"github.com/dividat/driver/src/dividat-driver/bluetooth"
	"github.com/muka/go-bluetooth/bluez/profile/device"
	"github.com/sirupsen/logrus"
	"go.bug.st/serial"
	"go.bug.st/serial/enumerator"
	"golang.org/x/sys/unix"
)

// Handle for managing SensingTex connection
type Handle struct {
	broker *pubsub.PubSub

	ctx context.Context

	cancelCurrentConnection context.CancelFunc
	subscriberCount         int

	log *logrus.Entry
}

// New returns an initialized handler
func New(ctx context.Context, log *logrus.Entry) *Handle {
	handle := Handle{
		broker: pubsub.New(32),
		ctx:    ctx,
		log:    log,
	}

	// Clean up
	go func() {
		<-ctx.Done()
		handle.broker.Shutdown()
	}()

	return &handle
}

// Connect to device
func (handle *Handle) Connect() {
	handle.subscriberCount++

	// If there is no existing connection, create it
	if handle.cancelCurrentConnection == nil {
		ctx, cancel := context.WithCancel(handle.ctx)

		onReceive := func(data []byte) {
			handle.broker.TryPub(data, "flex-rx")
		}

		go listeningLoop(ctx, handle.log, onReceive)

		handle.cancelCurrentConnection = cancel
	}
}

// Deregister subscribers and disconnect when none left
func (handle *Handle) DeregisterSubscriber() {
	handle.subscriberCount--

	if handle.subscriberCount == 0 && handle.cancelCurrentConnection != nil {
		handle.cancelCurrentConnection()
		handle.cancelCurrentConnection = nil
	}
}

// Keep looking for serial devices and connect to them when found, sending signals into the
// callback.
func listeningLoop(ctx context.Context, logger *logrus.Entry, onReceive func([]byte)) {
	for {
		scanAndConnectSerial(ctx, logger, onReceive)
		connectBluetooth(ctx, logger, onReceive)

		// Terminate if we were cancelled
		if ctx.Err() != nil {
			return
		}

		time.Sleep(2 * time.Second)
	}
}

// Serial

// One pass of browsing for serial devices and trying to connect to them turn by turn, first
// successful connection wins.
func scanAndConnectSerial(ctx context.Context, logger *logrus.Entry, onReceive func([]byte)) {
	ports, err := enumerator.GetDetailedPortsList()
	if err != nil {
		logger.WithError(err).Info("Could not list serial devices.")
		return
	}

	for _, port := range ports {
		// Terminate if we have been cancelled
		if ctx.Err() != nil {
			return
		}

		logger.WithField("name", port.Name).WithField("vendor", port.VID).Debug("Considering serial port.")

		if serialIsFlexLike(port) {
			measureWithSerialConnection(ctx, logger, onReceive, port.Name)
		}
	}
}

// Check whether a port looks like a potential Flex device.
//
// Vendor IDs:
//   16C0 - Van Ooijen Technische Informatica (Teensy)
func serialIsFlexLike(port *enumerator.PortDetails) bool {
	vendorId := strings.ToUpper(port.VID)

	return vendorId == "16C0"
}

// Connect to an individual serial port and start the measurement
func measureWithSerialConnection(
	ctx context.Context,
	logger *logrus.Entry,
	onReceive func([]byte),
	serialName string,
) {
	mode := &serial.Mode{
		BaudRate: 115200,
		Parity:   serial.NoParity,
		DataBits: 8,
		StopBits: serial.OneStopBit,
	}

	logger.WithField("name", serialName).Info("Attempting to connect with serial port.")
	port, err := serial.Open(serialName, mode)
	if err != nil {
		logger.WithField("config", mode).WithError(err).Info("Failed to open connection to serial port.")
		return
	}
	defer func() {
		logger.WithField("name", serialName).Info("Disconnecting from serial port.")
		port.Close()
	}()

	reader := bufio.NewReader(port)
	readByte := func() (byte, error) {
		return reader.ReadByte()
	}

	write := func(bytes []byte) error {
		_, err = port.Write(bytes)
		return err
	}

	logger.Info("Starting Senso Flex measurement with serial port.")
	measure(ctx, logger, onReceive, readByte, write)
}

// Bluetooth

// List connected bluetooth devices and try to connect to them turn by turn,
// first successful connection wins.
func connectBluetooth(ctx context.Context, logger *logrus.Entry, onReceive func([]byte)) {
	for devPath, dev := range bluetooth.GetConnectedDevices() {
		// Terminate if we have been cancelled
		if ctx.Err() != nil {
			return
		}

		if bluetoothIsFlexLike(dev) {
			measureWithBluetoothConnection(ctx, logger, onReceive, devPath, dev)
		}
	}

	// Flushing devices, otherwise a reconnected device can miss to connect
	bluetooth.FlushDevices()
}

func bluetoothIsFlexLike(dev *device.Device1) bool {
	return strings.HasPrefix(dev.Properties.Name, "DIVIDAT_")
}

// Connect via bluetooth and start the measurement
func measureWithBluetoothConnection(
	ctx context.Context,
	logger *logrus.Entry,
	onReceive func([]byte),
	devPath string,
	dev *device.Device1,
) {
	if !dev.Properties.Paired {
		logger.Infof("Pairing device %s", dev.Properties.Name)
		err := bluetooth.Pair(devPath, dev)
		if err != nil {
			logger.WithError(err).Error("Error pairing bluetooth device.")
			return
		}
	}

	fd, err := unix.Socket(syscall.AF_BLUETOOTH, syscall.SOCK_STREAM, unix.BTPROTO_RFCOMM)
	if err != nil {
		logger.WithError(err).Error("Error creating socket for the Bluetooth device.")
		return
	}
	defer unix.Close(fd)

	addr := &unix.SockaddrRFCOMM{Addr: str2ba(dev.Properties.Address), Channel: 1}
	err = unix.Connect(fd, addr)
	if err != nil {
		logger.WithError(err).Error("Error connecting to socket for the Bluetooth device.")
		return
	}

	readByte := func() (byte, error) {
		data := make([]byte, 1)
		n, err := unix.Read(fd, data)
		if err != nil {
			return ' ', err
		} else if n != 1 {
			return ' ', errors.New(fmt.Sprintf("Received %d bytes instead of 1.", n))
		} else {
			return data[0], nil
		}
	}

	write := func(bytes []byte) error {
		_, err = unix.Write(fd, bytes)
		return err
	}

	logger.Info("Starting Senso Flex measurement with Bluetooth.")
	measure(ctx, logger, onReceive, readByte, write)

	// Forcing the current device to be cleaned up
	bluetooth.FlushDevices()
}

// str2ba converts MAC address string representation to little-endian byte array
func str2ba(addr string) [6]byte {
	a := strings.Split(addr, ":")
	var b [6]byte
	for i, tmp := range a {
		u, _ := strconv.ParseUint(tmp, 16, 8)
		b[len(b)-1-i] = byte(u)
	}
	return b
}

// Pipe flex signal into the callback, summarizing package units into a buffer.

type ReaderState int

const (
	WAITING_FOR_HEADER ReaderState = iota
	HEADER_START
	HEADER_READ_LENGTH_MSB
	WAITING_FOR_BODY
	BODY_START
	BODY_READ_SAMPLE
	UNEXPECTED_BYTE
)

const (
	HEADER_START_MARKER = 'N'
	BODY_START_MARKER   = 'P'
	BYTES_PER_SAMPLE    = 4
)

func measure(
	ctx context.Context,
	logger *logrus.Entry,
	onReceive func([]byte),
	readByte func() (byte, error),
	write func([]byte) error,
) {
	START_MEASUREMENT_CMD := []byte{'S', '\n'}

	err := write(START_MEASUREMENT_CMD)
	if err != nil {
		logger.WithError(err).Info("Failed to write start message during flex measurement.")
		return
	}

	state := WAITING_FOR_HEADER
	var samplesLeftInSet int
	var bytesLeftInSample int

	var buff []byte
	for {
		// Terminate if we were cancelled
		if ctx.Err() != nil {
			return
		}

		input, err := readByte()
		if err != nil {
			logger.WithError(err).Info("Error reading byte during flex measurement.")
			return
		}

		// Finite State Machine for parsing byte stream
		switch {
		case state == WAITING_FOR_HEADER && input == HEADER_START_MARKER:
			state = HEADER_START
		case state == HEADER_START && input == '\n':
			state = HEADER_READ_LENGTH_MSB
		case state == HEADER_READ_LENGTH_MSB:
			// The number of measurements in each set may vary and is
			// given as two consecutive bytes (big-endian).
			msb := input
			lsb, err := readByte()
			if err != nil {
				logger.WithError(err).Info("Error reading byte during flex measurement.")
				return
			}
			samplesLeftInSet = int(binary.BigEndian.Uint16([]byte{msb, lsb}))
			state = WAITING_FOR_BODY
		case state == WAITING_FOR_BODY && input == BODY_START_MARKER:
			state = BODY_START
		case state == BODY_START && input == '\n':
			state = BODY_READ_SAMPLE
			buff = []byte{}
			bytesLeftInSample = BYTES_PER_SAMPLE
		case state == BODY_READ_SAMPLE:
			buff = append(buff, input)
			bytesLeftInSample = bytesLeftInSample - 1

			if bytesLeftInSample <= 0 {
				samplesLeftInSet = samplesLeftInSet - 1

				if samplesLeftInSet <= 0 {
					// Finish and send set
					onReceive(buff)

					// Get ready for next set and request it
					state = WAITING_FOR_HEADER
					err = write(START_MEASUREMENT_CMD)
					if err != nil {
						logger.WithError(err).Info("Failed to write poll message during flex measurement.")
						return
					}
				} else {
					// Start next point
					bytesLeftInSample = BYTES_PER_SAMPLE
				}
			}
		case state == UNEXPECTED_BYTE && input == HEADER_START_MARKER:
			// Recover from error state when a new header is seen
			state = HEADER_START
		default:
			state = UNEXPECTED_BYTE
		}

	}

}
