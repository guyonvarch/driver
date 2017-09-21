#! /usr/bin/env node
const fs = require('fs')
const path = require('path')
const pkg = require('../package.json')
const util = require('./util')
const packager = require('electron-packager')

packager(
  {
    platform: 'darwin,win32',
    arch: 'x64,ia32',
    icon: 'src/icons/dividat-icon',
    dir: '.',
    out: 'build',
    prune: true,
    packageManager: 'yarn',
    appCopyright: `(C) ${new Date().getFullYear()} ${pkg.author}`,
    win32metadata: {
      CompanyName: pkg.author,
      ProductName: pkg.productName,
      InternalName: pkg.productName,
      FileDescription: pkg.productName,
      OriginalFilename: `${pkg.productName}.exe`
    },
    afterCopy: [setChannel]
  },
    (err, appPaths) => {
      if (err) {
        console.error(`Error building Electron app: ${err.message}`)
        process.exit(1)
      }
    }
)

function setChannel (tmpDirPath, _, _, _, done) {
  const constantsPath = path.join(tmpDirPath, 'src', 'constants.json')
  const constants = require(constantsPath)

  constants.RELEASE_CHANNEL = util.getChannel()
  fs.writeFileSync(constantsPath, JSON.stringify(constants))
  done()
}
