/************************************************
 * This is the express server that maps hardware
 * to WebSocket
 ************************************************/
const express = require('express')
const app = express()
const cors = require('cors')

const pkg = require('../package.json')

const log = require('electron-log')

const fs = require('fs')

const packageBase = process.resourcesPath != null
  ? (process.resourcesPath + '/app')
  : '.'

function factory (sensoAddress, recorder) {
  log.info('Dividat Driver (' + pkg.version + ') starting up...')

  // Load and connect Senso
  const senso = require('./senso')(sensoAddress, recorder)

  // Start the server

  const https = require('https')
  const atbash = require('./util/atbash')
  const server = https.createServer({
    key: atbash(fs.readFileSync(`${packageBase}/ssl/key.atbash`)),
    cert: fs.readFileSync(`${packageBase}/ssl/cert.pem`)
  }, app)
  server.listen(8380, function () {
    log.info('SERVER: Listening on ' + server.address().port)
  })

  server.on('error', (error) => {
    switch (error.code) {
      case 'EADDRINUSE':
        log.error('SERVER: Address already in use. Could not start listening.')
        break
      default:
        log.error('SERVER: Error ' + error.code)
        log.error(error)
        break
    }
  })

  // socket.io
  const io = require('socket.io')(server)

  app.use(cors({
    origin: [/dividat\.(com|ch)$/, 'http://localhost:8080']
  }))

    /************************************************
     * Index Route
     ************************************************/
  app.get('/', function (req, res) {
    res.json({
      message: 'Dividat Driver',
      version: pkg.version
    })
  })

    /************************************************
     * Debug Route
     ************************************************/
    // app.use('/debug', express.static('src/debug'));

    /************************************************
     * Handle WebSocket connections
     ************************************************/
  io.on('connection', senso)

  return server
}

module.exports = factory

if (require.main === module) {
  var argv = require('minimist')(process.argv.slice(2))

  let recorder
  if ('rec' in argv) {
    log.info('Recording data to: ' + argv['rec'])
    const fs = require('fs')
    recorder = fs.createWriteStream(argv['rec'])
  }

  let sensoAddress
  if ('address' in argv) {
    sensoAddress = argv['address']
  }

  factory(sensoAddress, recorder)
}
