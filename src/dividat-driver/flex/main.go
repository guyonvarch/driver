package flex

/* Connects to Senso Flex devices through a serial connection and combines serial data into measurement sets.

This helps establish an indirect WebSocket connection to receive a stream of samples from the device.

The functionality of this module is as follows:

- While connected, scan for serial devices that look like a potential Flex device
- Connect to suitable serial devices and start polling for measurements
- Minimally parse incoming data to determine start and end of a measurement
- Send each complete measurement set to client as a binary package

*/

import (
	"bufio"
	"context"
	"encoding/binary"
	"strings"
	"time"

	"github.com/cskr/pubsub"
	"github.com/sirupsen/logrus"
	"go.bug.st/serial"
	"go.bug.st/serial/enumerator"
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

		// Terminate if we were cancelled
		if ctx.Err() != nil {
			return
		}

		time.Sleep(2 * time.Second)
	}
}

// One pass of browsing for serial devices and trying to connect to them turn by turn, first
// successful connection wins.
func scanAndConnectSerial(ctx context.Context, logger *logrus.Entry, onReceive func([]byte)) {
	ports, err := enumerator.GetDetailedPortsList()
	if err != nil {
		logger.WithField("error", err).Info("Could not list serial devices.")
		return
	}

	for _, port := range ports {
		// Terminate if we have been cancelled
		if ctx.Err() != nil {
			return
		}

		logger.WithField("name", port.Name).WithField("vendor", port.VID).Debug("Considering serial port.")

		if isFlexLike(port) {
			connectSerial(ctx, logger, port.Name, onReceive)
		}
	}
}

// Check whether a port looks like a potential Flex device.
//
// Vendor IDs:
//   16C0 - Van Ooijen Technische Informatica (Teensy)
func isFlexLike(port *enumerator.PortDetails) bool {
	vendorId := strings.ToUpper(port.VID)

	return vendorId == "16C0"
}

// Serial communication

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

// Actually attempt to connect to an individual serial port and pipe its signal into the callback, summarizing
// package units into a buffer.
func connectSerial(ctx context.Context, logger *logrus.Entry, serialName string, onReceive func([]byte)) {
	mode := &serial.Mode{
		BaudRate: 115200,
		Parity:   serial.NoParity,
		DataBits: 8,
		StopBits: serial.OneStopBit,
	}

	START_MEASUREMENT_CMD := []byte{'S', '\n'}

	logger.WithField("name", serialName).Info("Attempting to connect with serial port.")
	port, err := serial.Open(serialName, mode)
	if err != nil {
		logger.WithField("config", mode).WithField("error", err).Info("Failed to open connection to serial port.")
		return
	}
	defer func() {
		logger.WithField("name", serialName).Info("Disconnecting from serial port.")
		port.Close()
	}()

	_, err = port.Write(START_MEASUREMENT_CMD)
	if err != nil {
		logger.WithField("error", err).Info("Failed to write start message to serial port.")
		return
	}

	reader := bufio.NewReader(port)
	state := WAITING_FOR_HEADER
	var samplesLeftInSet int
	var bytesLeftInSample int

	var buff []byte
	for {
		// Terminate if we were cancelled
		if ctx.Err() != nil {
			return
		}

		input, err := reader.ReadByte()
		if err != nil {
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
			lsb, err := reader.ReadByte()
			if err != nil {
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
					_, err = port.Write(START_MEASUREMENT_CMD)
					if err != nil {
						logger.WithField("error", err).Info("Failed to write poll message to serial port.")
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
