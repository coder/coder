/**
 * @description
 * HTTP code snippet generator for R using httr
 *
 * @author
 * @gabrielakoreeda
 *
 * for any questions or issues regarding the generated code snippet, please open an issue mentioning the author.
 */

'use strict'

var util = require('util')
var CodeBuilder = require('../../helpers/code-builder')

module.exports = function (source, options) {
  // Start snippet
  var code = new CodeBuilder()

  // Import httr
  code.push('library(httr)')
    .blank()

  // Set URL
  code.push('url <- "%s"', source.url)
    .blank()

  // Construct query string
  var query
  var qs = source.queryObj
  var queryCount = Object.keys(qs).length
  delete source.queryObj['key']

  if (source.queryString.length === 1) {
    code.push('queryString <- list(%s = "%s")', Object.keys(qs), Object.values(qs).toString())
      .blank()
  } else if (source.queryString.length > 1) {
    var count = 1

    code.push('queryString <- list(')

    for (query in qs) {
      if (count++ !== queryCount - 1) {
        code.push('  %s = "%s",', query, qs[query].toString())
      } else {
        code.push('  %s = "%s"', query, qs[query].toString())
      }
    }

    code.push(')')
      .blank()
  }

  // Construct payload
  var payload = JSON.stringify(source.postData.text)

  if (payload) {
    code.push('payload <- %s', payload)
      .blank()
  }

  // Define encode
  if (source.postData.text || source.postData.jsonObj || source.postData.params) {
    switch (source.postData.mimeType) {
      case 'application/x-www-form-urlencoded':
        code.push('encode <- "form"')
          .blank()
        break

      case 'application/json':
        code.push('encode <- "json"')
          .blank()
        break

      case 'multipart/form-data':
        code.push('encode <- "multipart"')
          .blank()
        break

      default:
        code.push('encode <- "raw"')
          .blank()
        break
    }
  }

  // Construct headers
  var head
  var headers = source.allHeaders
  var headerCount = Object.keys(headers).length
  var header = ''
  var cookies
  var accept

  for (head in headers) {
    if (head.toLowerCase() === 'accept') {
      accept = ', accept("' + headers[head] + '")'
      headerCount = headerCount - 1
    } else if (head.toLowerCase() === 'cookie') {
      cookies = ', set_cookies(`' + headers[head].replace(/;/g, '", `').replace(/` /g, '`').replace(/=/g, '` = "') + '")'
      headerCount = headerCount - 1
    } else if (head.toLowerCase() !== 'content-type') {
      header = header + head.replace('-', '_') + " = '" + headers[head]
      if (headerCount > 1) { header = header + "', " }
    }
  }

  // Construct request
  var method = source.method
  var request = util.format('response <- VERB("%s", url', method)

  if (payload) {
    request += ', body = payload'
  }

  if (header !== '') {
    request += ', add_headers(' + header + "')"
  }

  if (source.queryString.length) {
    request += ', query = queryString'
  }

  request += ', content_type("' + source.postData.mimeType + '")'

  if (typeof accept !== 'undefined') {
    request += accept
  }

  if (typeof cookies !== 'undefined') {
    request += cookies
  }

  if (source.postData.text || source.postData.jsonObj || source.postData.params) {
    request += ', encode = encode'
  }

  request += ')'

  code.push(request)
    .blank()

  // Print response
    .push('content(response, "text")')

  return code.join()
}

module.exports.info = {
  key: 'httr',
  title: 'httr',
  link: 'https://cran.r-project.org/web/packages/httr/vignettes/quickstart.html',
  description: 'httr: Tools for Working with URLs and HTTP'
}
