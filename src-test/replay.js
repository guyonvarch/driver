const fs = require('fs')
const split = require('binary-split')
const net = require('net')

const bonjour = require('bonjour')()

bonjour.publish({name: 'Senso data replayer', txt: {ser_no: '01-23456789'}, type: 'sensoControl', port: '55567'})

const EventEmitter = require('events')

var HOST = '0.0.0.0'
var PORT = 55568

module.exports = function (recFile, timeout) {
  var replayer = Replayer(recFile, timeout)

  net.createServer(function (socket) {
        // We have a connection - a socket object is assigned to the connection automatically
    console.log('DATA - Connection: ' + socket.remoteAddress + ':' + socket.remotePort)

    function send (data) {
      socket.write(data)
    }

    replayer.on('data', send)

    socket.on('close', () => {
      replayer.removeListener('data', send)
      console.log('DATA - Closed: ' + socket.remoteAddress + ' ' + socket.remotePort)
    })

    socket.on('error', (err) => {
      console.log('Error: ', err)
    })
  }).listen(PORT, HOST)
  console.log('DATA listening on ' + HOST + ':' + PORT)
}

function Replayer (recFile, timeout) {
  var emitter = new EventEmitter()

  function createStream () {
    var stream = new fs.createReadStream(recFile).pipe(split())

    stream.on('data', (data) => {
      stream.pause()

      var buf = new Buffer(data.toString(), 'base64')
      emitter.emit('data', buf)

      setTimeout(() => {
        stream.resume()
      }, timeout)
    }).on('end', () => {
      createStream()
    })
  }

  createStream()

  return emitter
}
