/* Senso bridge
 * Creates a TCP <-> WebSocket bridge for connecting Play to Senso via a TCP connection.
 */
const Connection = require('./persistentConnection')

const DATA_PORT = 55568
const CONTROL_PORT = 55567
const DEFAULT_SENSO_ADDRESS = '169.254.1.10'

const log = require('electron-log')

// TODO: Think about removing configuration on Driver side. If Zeroconf works perfectly no configuration should be needed and otherwise it maybe should be stored on Play side.
let config
const constants = require('./constants')
try {
  const Config = require('electron-config')
  config = new Config()
} catch (err) {
  log.warn('Could not load config file.')
  config = {
    get: function (key) {
      return
    },
    set: function (key, value) {
      return
    }
  }
}

const discovery = require('./Senso/discovery')(log)

module.exports = (sensoAddress, recorder) => {
  sensoAddress = sensoAddress || config.get(constants.SENSO_ADDRESS_KEY) || DEFAULT_SENSO_ADDRESS

  var dataConnection = new Connection(sensoAddress, DATA_PORT, 'DATA', log)
  var controlConnection = new Connection(sensoAddress, CONTROL_PORT, 'CONTROL', log)

    // set up recording
  if (recorder) {
    dataConnection.on('data', (data) => {
      recorder.write(data.toString('base64'))
      recorder.write('\n')
    })
  }

  dataConnection.on('error', (err) => {
    log.verbose('DATA: connection error', err)
  })
  controlConnection.on('error', (err) => {
    log.verbose('CONTROL: connection error', err)
  })

  // connect with predefined default
  connect(sensoAddress)

  // On Linux installations, connect to any Senso discovered
  if (process.platform === 'linux') {
    discovery.on('found', connect)
  }

  function connect (address) {
    log.info('SENSO: Connecting to ' + address)
    dataConnection.connect(address)
    controlConnection.connect(address)
  }

  function onPlayConnection (ws) {
        // Handle a new connection to the application (most probably Play, but could be anything else ... like Manager)
        // argument is a socket.io socket (https://socket.io/docs/server-api/)

    log.info('Play connected (session:', ws.id, ')')

        // Create a send function so that it can be cleanly removed from the dataEmitter
    function sendData (data) {
      ws.emit('DataRaw', data)
    }
    dataConnection.on('data', sendData)

    function sendControl (data) {
      ws.emit('ControlRaw', data)
    }
    controlConnection.on('data', sendControl)

    function sendSensoConnection () {
      ws.emit('BridgeMessage', {
        type: 'SensoConnection',
        connected: (controlConnection.connected && dataConnection.connected),
        connection: {
          type: 'IP',
          address: dataConnection.host
        }
      })
    }
    dataConnection.on('connect', sendSensoConnection)
    dataConnection.on('close', sendSensoConnection)
    controlConnection.on('connect', sendSensoConnection)
    controlConnection.on('close', sendSensoConnection)
    sendSensoConnection()

    function onTimeout () {
      log.verbose('CONTROL: TCP timeout.')
      ws.emit('BridgeMessage', {
        type: 'SensoConnectionTimeout'
      })
    }
    controlConnection.on('timeout', onTimeout)

    // Forward the discovery of (additional) Sensos to Play
    discovery.on('found', (address) => {
      log.verbose('mDNS: Found Senso at ' + address)
      ws.emit('BridgeMessage', {
        type: 'SensoDiscovered',
        connection: {
          type: 'IP',
          address: address
        }
      })
    })

    ws.on('SendControlRaw', (data) => {
      try {
        var socket = controlConnection.getSocket()
        log.debug('CONTROL: ', data)
        if (socket) {
          socket.write(data)
        } else {
          log.warn('CONTROL: Can not send command to Senso, no connection.')
        }
      } catch (e) {
        log.error('CONTROL: Error while handling Command:', e)
      }
    })

    ws.on('BridgeCommand', (command) => {
      try {
        switch (command.type) {
          case 'SensoConnect':
            if (command.connection.type === 'IP' && command.connection.address) {
              config.set(constants.SENSO_ADDRESS_KEY, command.connection.address)
              connect(command.connection.address)
            }
            break
          case 'GetSensoConnection':
            sendSensoConnection()
            break

          default:
            break
        }
      } catch (e) {
        log.error('Error while handling BridgeCommand:', e)
      }
    })

    // handle disconnect
    ws.on('disconnect', () => {
      log.info('WS: Disconnected.')

      dataConnection.removeListener('data', sendData)
      dataConnection.removeListener('connect', sendSensoConnection)
      dataConnection.removeListener('close', sendSensoConnection)

      controlConnection.removeListener('data', sendControl)
      controlConnection.removeListener('connect', sendSensoConnection)
      controlConnection.removeListener('close', sendSensoConnection)
      controlConnection.removeListener('timeout', onTimeout)
    })
  }

  return onPlayConnection
}
