const rotations = [
  ['a', 'z'], ['A', 'Z'], ['0', '9']
].map(cs => cs.map(c => c.charCodeAt(0)))

module.exports = function atbash (buffer) {
  if (typeof buffer === 'string') {
    buffer = Buffer.from(buffer)
  }

  for (let ix = 0, len = buffer.length; ix < len; ix++) {
    let code = buffer.readUInt8(ix)
    for (let [lower, upper] of rotations) {
      if (code >= lower && code <= upper) {
        buffer.writeUInt8(upper - code + lower, ix)
      }
    }
  }

  return buffer
}
