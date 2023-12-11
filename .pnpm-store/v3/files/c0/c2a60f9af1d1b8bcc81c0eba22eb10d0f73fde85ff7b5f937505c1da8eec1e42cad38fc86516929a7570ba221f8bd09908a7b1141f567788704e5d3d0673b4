/**
 * @description
 * HTTP code snippet generator to generate raw HTTP/1.1 request strings,
 * in accordance to the RFC 7230 (and RFC 7231) specifications.
 *
 * @author
 * @irvinlim
 *
 * For any questions or issues regarding the generated code snippet, please open an issue mentioning the author.
 */

'use strict'

var CRLF = '\r\n'
var CodeBuilder = require('../../helpers/code-builder')
var util = require('util')

/**
 * Request follows the request message format in accordance to RFC 7230, Section 3.
 * Each section is prepended with the RFC and section number.
 * See more at https://tools.ietf.org/html/rfc7230#section-3.
 */
module.exports = function (source, options) {
  var opts = Object.assign(
    {
      absoluteURI: false,
      autoContentLength: true,
      autoHost: true
    },
    options
  )

  // RFC 7230 Section 3. Message Format
  // All lines have no indentation, and should be terminated with CRLF.
  var code = new CodeBuilder('', CRLF)

  // RFC 7230 Section 5.3. Request Target
  // Determines if the Request-Line should use 'absolute-form' or 'origin-form'.
  // Basically it means whether the "http://domain.com" will prepend the full url.
  var requestUrl = opts.absoluteURI ? source.fullUrl : source.uriObj.path

  // RFC 7230 Section 3.1.1. Request-Line
  code.push('%s %s %s', source.method, requestUrl, source.httpVersion)

  // RFC 7231 Section 5. Header Fields
  Object.keys(source.allHeaders).forEach(function (key) {
    // Capitalize header keys, even though it's not required by the spec.
    var keyCapitalized = key.toLowerCase().replace(/(^|-)(\w)/g, function (x) {
      return x.toUpperCase()
    })

    code.push(
      '%s',
      util.format('%s: %s', keyCapitalized, source.allHeaders[key])
    )
  })

  // RFC 7230 Section 5.4. Host
  // Automatically set Host header if option is on and on header already exists.
  if (opts.autoHost && Object.keys(source.allHeaders).indexOf('host') === -1) {
    code.push('Host: %s', source.uriObj.host)
  }

  // RFC 7230 Section 3.3.3. Message Body Length
  // Automatically set Content-Length header if option is on, postData is present and no header already exists.
  if (
    opts.autoContentLength &&
    source.postData.text &&
    Object.keys(source.allHeaders).indexOf('content-length') === -1
  ) {
    code.push(
      'Content-Length: %d',
      Buffer.byteLength(source.postData.text, 'ascii')
    )
  }

  // Add extra line after header section.
  code.blank()

  // Separate header section and message body section.
  var headerSection = code.join()
  var messageBody = ''

  // RFC 7230 Section 3.3. Message Body
  if (source.postData.text) {
    messageBody = source.postData.text
  }

  // RFC 7230 Section 3. Message Format
  // Extra CRLF separating the headers from the body.
  return headerSection + CRLF + messageBody
}

module.exports.info = {
  key: '1.1',
  title: 'HTTP/1.1',
  link: 'https://tools.ietf.org/html/rfc7230',
  description: 'HTTP/1.1 request string in accordance with RFC 7230'
}
