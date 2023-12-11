/**
 * @description
 * HTTP code snippet generator for the Shell using cURL.
 *
 * @author
 * @AhmadNassri
 *
 * for any questions or issues regarding the generated code snippet, please open an issue mentioning the author.
 */

'use strict'

var util = require('util')
var helpers = require('../../helpers/shell')
var CodeBuilder = require('../../helpers/code-builder')

module.exports = function (source, options) {
  var opts = Object.assign({
    indent: '  ',
    short: false,
    binary: false
  }, options)

  var code = new CodeBuilder(opts.indent, opts.indent !== false ? ' \\\n' + opts.indent : ' ')

  code.push('curl %s %s', opts.short ? '-X' : '--request', source.method)
      .push(util.format('%s%s', opts.short ? '' : '--url ', helpers.quote(source.fullUrl)))

  if (source.httpVersion === 'HTTP/1.0') {
    code.push(opts.short ? '-0' : '--http1.0')
  }

  // construct headers
  Object.keys(source.headersObj).sort().forEach(function (key) {
    var header = util.format('%s: %s', key, source.headersObj[key])
    code.push('%s %s', opts.short ? '-H' : '--header', helpers.quote(header))
  })

  if (source.allHeaders.cookie) {
    code.push('%s %s', opts.short ? '-b' : '--cookie', helpers.quote(source.allHeaders.cookie))
  }

  // construct post params
  switch (source.postData.mimeType) {
    case 'multipart/form-data':
      source.postData.params.map(function (param) {
        var post = ''
        if (param.fileName) {
          post = util.format('%s=@%s', param.name, param.fileName)
        } else {
          post = util.format('%s=%s', param.name, param.value)
        }

        code.push('%s %s', opts.short ? '-F' : '--form', helpers.quote(post))
      })
      break

    case 'application/x-www-form-urlencoded':
      if (source.postData.params) {
        source.postData.params.map(function (param) {
          code.push(
            '%s %s', opts.binary ? '--data-binary' : (opts.short ? '-d' : '--data'),
            helpers.quote(util.format('%s=%s', param.name, param.value))
          )
        })
      } else {
        code.push(
          '%s %s', opts.binary ? '--data-binary' : (opts.short ? '-d' : '--data'),
          helpers.quote(source.postData.text)
        )
      }
      break

    default:
      // raw request body
      if (source.postData.text) {
        code.push(
          '%s %s', opts.binary ? '--data-binary' : (opts.short ? '-d' : '--data'),
          helpers.quote(source.postData.text)
        )
      }
  }

  return code.join()
}

module.exports.info = {
  key: 'curl',
  title: 'cURL',
  link: 'http://curl.haxx.se/',
  description: 'cURL is a command line tool and library for transferring data with URL syntax'
}
