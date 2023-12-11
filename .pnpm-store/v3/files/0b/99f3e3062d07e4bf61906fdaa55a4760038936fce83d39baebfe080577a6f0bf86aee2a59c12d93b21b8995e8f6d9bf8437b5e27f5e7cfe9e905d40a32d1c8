/**
 * @description
 * HTTP code snippet generator for Python using Requests
 *
 * @author
 * @montanaflynn
 *
 * for any questions or issues regarding the generated code snippet, please open an issue mentioning the author.
 */

'use strict'

var util = require('util')
var CodeBuilder = require('../../helpers/code-builder')
var helpers = require('./helpers')

module.exports = function (source, options) {
  var opts = Object.assign({
    indent: '    ',
    pretty: true
  }, options)

  // Start snippet
  var code = new CodeBuilder('    ')

  // Import requests
  code.push('import requests')
      .blank()

  // Set URL
  code.push('url = "%s"', source.url)
      .blank()

  // Construct query string
  if (Object.keys(source.queryObj).length) {
    var qs = 'querystring = ' + JSON.stringify(source.queryObj)

    code.push(qs)
        .blank()
  }

  // Construct payload
  let hasPayload = false
  let jsonPayload = false
  switch (source.postData.mimeType) {
    case 'application/json':
      if (source.postData.jsonObj) {
        code.push('payload = %s', helpers.literalRepresentation(source.postData.jsonObj, opts))
        jsonPayload = true
        hasPayload = true
      }
      break

    default:
      var payload = JSON.stringify(source.postData.text)
      if (payload) {
        code.push('payload = %s', payload)
        hasPayload = true
      }
  }

  // Construct headers
  var header
  var headers = source.allHeaders
  var headerCount = Object.keys(headers).length

  if (headerCount === 1) {
    for (header in headers) {
      code.push('headers = {"%s": "%s"}', header, headers[header])
          .blank()
    }
  } else if (headerCount > 1) {
    var count = 1

    code.push('headers = {')

    for (header in headers) {
      if (count++ !== headerCount) {
        code.push(1, '"%s": "%s",', header, headers[header])
      } else {
        code.push(1, '"%s": "%s"', header, headers[header])
      }
    }

    code.push('}')
        .blank()
  }

  // Construct request
  var method = source.method
  var request = util.format('response = requests.request("%s", url', method)

  if (hasPayload) {
    if (jsonPayload) {
      request += ', json=payload'
    } else {
      request += ', data=payload'
    }
  }

  if (headerCount > 0) {
    request += ', headers=headers'
  }

  if (qs) {
    request += ', params=querystring'
  }

  request += ')'

  code.push(request)
      .blank()

      // Print response
      .push('print(response.text)')

  return code.join()
}

module.exports.info = {
  key: 'requests',
  title: 'Requests',
  link: 'http://docs.python-requests.org/en/latest/api/#requests.request',
  description: 'Requests HTTP library'
}
