/**
 * @description
 * Asynchronous Http and WebSocket Client library for Java
 *
 * @author
 * @windard
 *
 * for any questions or issues regarding the generated code snippet, please open an issue mentioning the author.
 */

'use strict'

var CodeBuilder = require('../../helpers/code-builder')

module.exports = function (source, options) {
  var opts = Object.assign({
    indent: '  '
  }, options)

  var code = new CodeBuilder(opts.indent)

  code.push('AsyncHttpClient client = new DefaultAsyncHttpClient();')

  code.push(`client.prepare("${source.method.toUpperCase()}", "${source.fullUrl}")`)

  // Add headers, including the cookies
  var headers = Object.keys(source.allHeaders)

  // construct headers
  if (headers.length) {
    headers.forEach(function (key) {
      code.push(1, '.setHeader("%s", "%s")', key, source.allHeaders[key])
    })
  }

  if (source.postData.text) {
    code.push(1, '.setBody(%s)', JSON.stringify(source.postData.text))
  }

  code.push(1, '.execute()')
  code.push(1, '.toCompletableFuture()')
  code.push(1, '.thenAccept(System.out::println)')
  code.push(1, '.join();')
  code.blank()
  code.push('client.close();')

  return code.join()
}

module.exports.info = {
  key: 'asynchttp',
  title: 'AsyncHttp',
  link: 'https://github.com/AsyncHttpClient/async-http-client',
  description: 'Asynchronous Http and WebSocket Client library for Java'
}
