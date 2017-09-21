/* Performs a simple check for SSL certificate health.

    node check-cert.js path/to/cert.pem
*/
const child_process = require('child_process')
const fs = require('fs')
const tmp = require('tmp')

const certPath = process.argv[2]
if (certPath == null) {
  console.error('No certificate given.')
  process.exit(1)
}

// Strip the first cert to yield chain of intermediate -> root certs
const issuerCert = fs
  .readFileSync(certPath, 'utf8')
  .replace(/^-----BEGIN CERTIFICATE-----[\s\S]+?-----END CERTIFICATE-----$/m, '')
  .trim()
// Find the OCSP endpoint for this cert
const ocspUri = sslQuery(['x509', '-noout', '-ocsp_uri', '-in', certPath])

tmp.file((err, issuerCertPath, fd, cleanup) => {
  if (err) throw err

  // Create intermediate cert file
  fs.writeSync(fd, issuerCert)

  const report = sslQuery(['ocsp', '-issuer', issuerCertPath, '-cert', certPath, '-text', '-url', ocspUri])

  cleanup()
  if (!/^.*: good$/m.test(report)) {
    console.error('Certificate is not \'good\':\n')
    console.error(report)
    process.exit(2)
  } else {
    console.log('Certificate is \'good\'.')
    process.exit(0)
  }
})

function sslQuery (args) {
  const res = child_process.spawnSync('openssl', args, { encoding: 'utf8' })

  if (res.error != null) throw res.error

  return res.stdout.trim()
}
