/**
 * @description
 * HTTP code snippet generator for Node.js using Request.
 *
 * @author
 * @AhmadNassri
 *
 * for any questions or issues regarding the generated code snippet, please open an issue mentioning the author.
 */

'use strict'

var util = require('util')
var stringifyObject = require('stringify-object')
var CodeBuilder = require('../../helpers/code-builder')

module.exports = function (source, options) {
  var opts = Object.assign({
    indent: '  '
  }, options)

  var includeFS = false
  var code = new CodeBuilder(opts.indent)

  code.push("const request = require('request');")
      .blank()

  var reqOpts = {
    method: source.method,
    url: source.url
  }

  if (Object.keys(source.queryObj).length) {
    reqOpts.qs = source.queryObj
  }

  if (Object.keys(source.headersObj).length) {
    reqOpts.headers = source.headersObj
  }

  switch (source.postData.mimeType) {
    case 'application/x-www-form-urlencoded':
      reqOpts.form = source.postData.paramsObj
      break

    case 'application/json':
      if (source.postData.jsonObj) {
        reqOpts.body = source.postData.jsonObj
        reqOpts.json = true
      }
      break

    case 'multipart/form-data':
      reqOpts.formData = {}

      source.postData.params.forEach(function (param) {
        var attachment = {}

        if (!param.fileName && !param.fileName && !param.contentType) {
          reqOpts.formData[param.name] = param.value
          return
        }

        if (param.fileName) {
          includeFS = true

          attachment.value = 'fs.createReadStream("' + param.fileName + '")'
        } else if (param.value) {
          attachment.value = param.value
        }

        if (param.fileName) {
          attachment.options = {
            filename: param.fileName,
            contentType: param.contentType ? param.contentType : null
          }
        }

        reqOpts.formData[param.name] = attachment
      })
      break

    default:
      if (source.postData.text) {
        reqOpts.body = source.postData.text
      }
  }

  // construct cookies argument
  if (source.cookies.length) {
    reqOpts.jar = 'JAR'

    code.push('const jar = request.jar();')

    var url = source.url

    source.cookies.forEach(function (cookie) {
      code.push("jar.setCookie(request.cookie('%s=%s'), '%s');", encodeURIComponent(cookie.name), encodeURIComponent(cookie.value), url)
    })
    code.blank()
  }

  if (includeFS) {
    code.unshift("const fs = require('fs');")
  }

  code.push('const options = %s;', stringifyObject(reqOpts, { indent: '  ', inlineCharacterLimit: 80 }))
    .blank()

  code.push(util.format('request(options, %s', 'function (error, response, body) {'))

      .push(1, 'if (error) throw new Error(error);')
      .blank()
      .push(1, 'console.log(body);')
      .push('});')
      .blank()

  return code.join().replace('"JAR"', 'jar').replace(/'fs\.createReadStream\("(.+)"\)'/g, "fs.createReadStream('$1')")
}

module.exports.info = {
  key: 'request',
  title: 'Request',
  link: 'https://github.com/request/request',
  description: 'Simplified HTTP request client'
}
