'use strict'

var Promise = require('pinkie-promise')
var fs = require('fs')

module.exports = function (filename, data, options) {
  return new Promise(function (_resolve, _reject) {
    fs.writeFile(filename, data, options, function (err) {
      return err === null ? _resolve(filename) : _reject(err)
    })
  })
  .catch(function (err) {
    throw err
  })
}
