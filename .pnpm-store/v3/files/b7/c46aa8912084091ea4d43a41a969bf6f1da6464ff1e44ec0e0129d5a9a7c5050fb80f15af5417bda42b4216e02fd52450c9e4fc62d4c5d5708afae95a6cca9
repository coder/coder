/**
 * @description
 * HTTP code snippet generator for native Node.js.
 *
 * @author
 * @AhmadNassri
 *
 * for any questions or issues regarding the generated code snippet, please open an issue mentioning the author.
 */

'use strict'

var stringifyObject = require('stringify-object')
var CodeBuilder = require('../../helpers/code-builder')

module.exports = function (source, options) {
  var opts = Object.assign({
    indent: '  '
  }, options)

  var code = new CodeBuilder(opts.indent)

  var reqOpts = {
    method: source.method,
    hostname: source.uriObj.hostname,
    port: source.uriObj.port,
    path: source.uriObj.path,
    headers: source.allHeaders
  }

  code.push('const http = require("%s");', source.uriObj.protocol.replace(':', ''))

  code.blank()
      .push('const options = %s;', JSON.stringify(reqOpts, null, opts.indent))
      .blank()
      .push('const req = http.request(options, function (res) {')
      .push(1, 'const chunks = [];')
      .blank()
      .push(1, 'res.on("data", function (chunk) {')
      .push(2, 'chunks.push(chunk);')
      .push(1, '});')
      .blank()
      .push(1, 'res.on("end", function () {')
      .push(2, 'const body = Buffer.concat(chunks);')
      .push(2, 'console.log(body.toString());')
      .push(1, '});')
      .push('});')
      .blank()

  switch (source.postData.mimeType) {
    case 'application/x-www-form-urlencoded':
      if (source.postData.paramsObj) {
        code.unshift('const qs = require("querystring");')
        code.push('req.write(qs.stringify(%s));', stringifyObject(source.postData.paramsObj, {
          indent: '  ',
          inlineCharacterLimit: 80
        }))
      }
      break

    case 'application/json':
      if (source.postData.jsonObj) {
        code.push('req.write(JSON.stringify(%s));', stringifyObject(source.postData.jsonObj, {
          indent: '  ',
          inlineCharacterLimit: 80
        }))
      }
      break

    default:
      if (source.postData.text) {
        code.push('req.write(%s);', JSON.stringify(source.postData.text, null, opts.indent))
      }
  }

  code.push('req.end();')

  return code.join()
}

module.exports.info = {
  key: 'native',
  title: 'HTTP',
  link: 'http://nodejs.org/api/http.html#http_http_request_options_callback',
  description: 'Node.js native HTTP interface'
}
