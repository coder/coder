/**
 * @description
 * HTTP code snippet generator for Clojure using clj-http.
 *
 * @author
 * @tggreene
 *
 * for any questions or issues regarding the generated code snippet, please open an issue mentioning the author.
 */

'use strict'

var CodeBuilder = require('../../helpers/code-builder')
var helpers = require('../../helpers/headers')

var Keyword = function (name) {
  this.name = name
}

Keyword.prototype.toString = function () {
  return ':' + this.name
}

var File = function (path) {
  this.path = path
}

File.prototype.toString = function () {
  return '(clojure.java.io/file "' + this.path + '")'
}

var jsType = function (x) {
  return (typeof x !== 'undefined')
          ? x.constructor.name.toLowerCase()
          : null
}

var objEmpty = function (x) {
  return (jsType(x) === 'object')
          ? Object.keys(x).length === 0
          : false
}

var filterEmpty = function (m) {
  Object.keys(m)
        .filter(function (x) { return objEmpty(m[x]) })
        .forEach(function (x) { delete m[x] })
  return m
}

var padBlock = function (x, s) {
  var padding = Array.apply(null, Array(x))
                    .map(function (_) {
                      return ' '
                    })
                    .join('')
  return s.replace(/\n/g, '\n' + padding)
}

var jsToEdn = function (js) {
  switch (jsType(js)) {
    default: // 'number' 'boolean'
      return js.toString()
    case 'string':
      return '"' + js.replace(/"/g, '\\"') + '"'
    case 'file':
      return js.toString()
    case 'keyword':
      return js.toString()
    case 'null':
      return 'nil'
    case 'regexp':
      return '#"' + js.source + '"'
    case 'object': // simple vertical format
      var obj = Object.keys(js)
                      .reduce(function (acc, key) {
                        var val = padBlock(key.length + 2, jsToEdn(js[key]))
                        return acc + ':' + key + ' ' + val + '\n '
                      }, '')
                      .trim()
      return '{' + padBlock(1, obj) + '}'
    case 'array': // simple horizontal format
      var arr = js.reduce(function (acc, val) {
        return acc + ' ' + jsToEdn(val)
      }, '').trim()
      return '[' + padBlock(1, arr) + ']'
  }
}

module.exports = function (source, options) {
  var code = new CodeBuilder(options)
  var methods = ['get', 'post', 'put', 'delete', 'patch', 'head', 'options']

  if (methods.indexOf(source.method.toLowerCase()) === -1) {
    return code.push('Method not supported').join()
  }

  var params = {headers: source.allHeaders,
    'query-params': source.queryObj}

  switch (source.postData.mimeType) {
    case 'application/json':
      params['content-type'] = new Keyword('json')
      params['form-params'] = source.postData.jsonObj
      delete params.headers[helpers.getHeaderName(params.headers, 'content-type')]
      break
    case 'application/x-www-form-urlencoded':
      params['form-params'] = source.postData.paramsObj
      delete params.headers[helpers.getHeaderName(params.headers, 'content-type')]
      break
    case 'text/plain':
      params.body = source.postData.text
      delete params.headers[helpers.getHeaderName(params.headers, 'content-type')]
      break
    case 'multipart/form-data':
      params.multipart = source.postData.params.map(function (x) {
        if (x.fileName && !x.value) {
          return {name: x.name,
            content: new File(x.fileName)}
        } else {
          return {name: x.name,
            content: x.value}
        }
      })
      delete params.headers[helpers.getHeaderName(params.headers, 'content-type')]
      break
  }

  switch (helpers.getHeader(params.headers, 'accept')) {
    case 'application/json':
      params.accept = new Keyword('json')
      delete params.headers[helpers.getHeaderName(params.headers, 'accept')]
      break
  }

  code.push('(require \'[clj-http.client :as client])\n')

  if (objEmpty(filterEmpty(params))) {
    code.push('(client/%s "%s")', source.method.toLowerCase(), source.url)
  } else {
    code.push('(client/%s "%s" %s)', source.method.toLowerCase(), source.url, padBlock(11 + source.method.length + source.url.length, jsToEdn(filterEmpty(params))))
  }

  return code.join()
}

module.exports.info = {
  key: 'clj_http',
  title: 'clj-http',
  link: 'https://github.com/dakrone/clj-http',
  description: 'An idiomatic clojure http client wrapping the apache client.'
}
