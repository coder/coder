'use strict'

var util = require('util')

/**
 * Create an string of given length filled with blank spaces
 *
 * @param {number} length Length of the array to return
 * @param {string} str String to pad out with
 */
function buildString (length, str) {
  return Array.apply(null, new Array(length)).map(String.prototype.valueOf, str).join('')
}

/**
 * Create a string corresponding to a Dictionary or Array literal representation with pretty option
 * and indentation.
 */
function concatValues (concatType, values, pretty, indentation, indentLevel) {
  var currentIndent = buildString(indentLevel, indentation)
  var closingBraceIndent = buildString(indentLevel - 1, indentation)
  var join = pretty ? ',\n' + currentIndent : ', '
  var openingBrace = concatType === 'object' ? '{' : '['
  var closingBrace = concatType === 'object' ? '}' : ']'

  if (pretty) {
    return openingBrace + '\n' + currentIndent + values.join(join) + '\n' + closingBraceIndent + closingBrace
  } else {
    return openingBrace + values.join(join) + closingBrace
  }
}

module.exports = {
  /**
   * Create a valid Python string of a literal value according to its type.
   *
   * @param {*} value Any JavaScript literal
   * @param {Object} opts Target options
   * @return {string}
   */
  literalRepresentation: function (value, opts, indentLevel) {
    indentLevel = indentLevel === undefined ? 1 : indentLevel + 1

    switch (Object.prototype.toString.call(value)) {
      case '[object Number]':
        return value

      case '[object Array]':
        var pretty = false
        var valuesRepresentation = value.map(function (v) {
          // Switch to prettify if the value is a dictionary with multiple keys
          if (Object.prototype.toString.call(v) === '[object Object]') {
            pretty = Object.keys(v).length > 1
          }
          return this.literalRepresentation(v, opts, indentLevel)
        }.bind(this))
        return concatValues('array', valuesRepresentation, pretty, opts.indent, indentLevel)

      case '[object Object]':
        var keyValuePairs = []
        for (var k in value) {
          keyValuePairs.push(util.format('"%s": %s', k, this.literalRepresentation(value[k], opts, indentLevel)))
        }
        return concatValues('object', keyValuePairs, opts.pretty && keyValuePairs.length > 1, opts.indent, indentLevel)

      case '[object Null]':
        return 'None'

      case '[object Boolean]':
        return value ? 'True' : 'False'

      default:
        if (value === null || value === undefined) {
          return ''
        }
        return '"' + value.toString().replace(/"/g, '\\"') + '"'
    }
  }
}
