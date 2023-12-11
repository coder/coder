/**
 * @description
 * HTTP code snippet generator for native XMLHttpRequest
 *
 * @author
 * @AhmadNassri
 *
 * for any questions or issues regarding the generated code snippet, please open an issue mentioning the author.
 */

'use strict'

var CodeBuilder = require('../../helpers/code-builder')
var helpers = require('../../helpers/headers')

module.exports = function (source, options) {
  var opts = Object.assign({
    indent: '  '
  }, options)

  var code = new CodeBuilder(opts.indent)

  var settings = {
    async: true,
    crossDomain: true,
    url: source.fullUrl,
    method: source.method,
    headers: source.allHeaders
  }

  switch (source.postData.mimeType) {
    case 'application/x-www-form-urlencoded':
      settings.data = source.postData.paramsObj ? source.postData.paramsObj : source.postData.text
      break

    case 'application/json':
      settings.processData = false
      settings.data = source.postData.text
      break

    case 'multipart/form-data':
      code.push('const form = new FormData();')

      source.postData.params.forEach(function (param) {
        code.push('form.append(%s, %s);', JSON.stringify(param.name), JSON.stringify(param.value || param.fileName || ''))
      })

      settings.processData = false
      settings.contentType = false
      settings.mimeType = 'multipart/form-data'
      settings.data = '[form]'

      // remove the contentType header
      if (helpers.hasHeader(settings.headers, 'content-type')) {
        if (helpers.getHeader(settings.headers, 'content-type').indexOf('boundary')) {
          delete settings.headers[helpers.getHeaderName(settings.headers, 'content-type')]
        }
      }

      code.blank()
      break

    default:
      if (source.postData.text) {
        settings.data = source.postData.text
      }
  }

  code.push('const settings = ' + JSON.stringify(settings, null, opts.indent).replace('"[form]"', 'form') + ';')
      .blank()
      .push('$.ajax(settings).done(function (response) {')
      .push(1, 'console.log(response);')
      .push('});')

  return code.join()
}

module.exports.info = {
  key: 'jquery',
  title: 'jQuery',
  link: 'http://api.jquery.com/jquery.ajax/',
  description: 'Perform an asynchronous HTTP (Ajax) requests with jQuery'
}
