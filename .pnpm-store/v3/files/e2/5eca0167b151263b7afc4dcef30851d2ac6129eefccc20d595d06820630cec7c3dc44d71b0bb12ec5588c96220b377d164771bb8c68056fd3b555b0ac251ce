(function (global, factory) {
	typeof exports === 'object' && typeof module !== 'undefined' ? module.exports = factory() :
	typeof define === 'function' && define.amd ? define(factory) :
	(global.codeErrorFragment = factory());
}(this, (function () { 'use strict';

/*!
 * repeat-string <https://github.com/jonschlinkert/repeat-string>
 *
 * Copyright (c) 2014-2015, Jon Schlinkert.
 * Licensed under the MIT License.
 */

'use strict';

/**
 * Results cache
 */

var res = '';
var cache;

/**
 * Expose `repeat`
 */

var repeatString = repeat;

/**
 * Repeat the given `string` the specified `number`
 * of times.
 *
 * **Example:**
 *
 * ```js
 * var repeat = require('repeat-string');
 * repeat('A', 5);
 * //=> AAAAA
 * ```
 *
 * @param {String} `string` The string to repeat
 * @param {Number} `number` The number of times to repeat the string
 * @return {String} Repeated string
 * @api public
 */

function repeat(str, num) {
  if (typeof str !== 'string') {
    throw new TypeError('expected a string');
  }

  // cover common, quick use cases
  if (num === 1) return str;
  if (num === 2) return str + str;

  var max = str.length * num;
  if (cache !== str || typeof cache === 'undefined') {
    cache = str;
    res = '';
  } else if (res.length >= max) {
    return res.substr(0, max);
  }

  while (max > res.length && num > 1) {
    if (num & 1) {
      res += str;
    }

    num >>= 1;
    str += str;
  }

  res += str;
  res = res.substr(0, max);
  return res;
}

'use strict';

var padStart = function (string, maxLength, fillString) {

  if (string == null || maxLength == null) {
    return string;
  }

  var result    = String(string);
  var targetLen = typeof maxLength === 'number'
    ? maxLength
    : parseInt(maxLength, 10);

  if (isNaN(targetLen) || !isFinite(targetLen)) {
    return result;
  }


  var length = result.length;
  if (length >= targetLen) {
    return result;
  }


  var fill = fillString == null ? '' : String(fillString);
  if (fill === '') {
    fill = ' ';
  }


  var fillLen = targetLen - length;

  while (fill.length < fillLen) {
    fill += fill;
  }

  var truncated = fill.length > fillLen ? fill.substr(0, fillLen) : fill;

  return truncated + result;
};

var _extends = Object.assign || function (target) {
  for (var i = 1; i < arguments.length; i++) {
    var source = arguments[i];

    for (var key in source) {
      if (Object.prototype.hasOwnProperty.call(source, key)) {
        target[key] = source[key];
      }
    }
  }

  return target;
};

function printLine(line, position, maxNumLength, settings) {
	var num = String(position);
	var formattedNum = padStart(num, maxNumLength, ' ');
	var tabReplacement = repeatString(' ', settings.tabSize);

	return formattedNum + ' | ' + line.replace(/\t/g, tabReplacement);
}

function printLines(lines, start, end, maxNumLength, settings) {
	return lines.slice(start, end).map(function (line, i) {
		return printLine(line, start + i + 1, maxNumLength, settings);
	}).join('\n');
}

var defaultSettings = {
	extraLines: 2,
	tabSize: 4
};

var index = (function (input, linePos, columnPos, settings) {
	settings = _extends({}, defaultSettings, settings);

	var lines = input.split(/\r\n?|\n|\f/);
	var startLinePos = Math.max(1, linePos - settings.extraLines) - 1;
	var endLinePos = Math.min(linePos + settings.extraLines, lines.length);
	var maxNumLength = String(endLinePos).length;
	var prevLines = printLines(lines, startLinePos, linePos, maxNumLength, settings);
	var targetLineBeforeCursor = printLine(lines[linePos - 1].substring(0, columnPos - 1), linePos, maxNumLength, settings);
	var cursorLine = repeatString(' ', targetLineBeforeCursor.length) + '^';
	var nextLines = printLines(lines, linePos, endLinePos, maxNumLength, settings);

	return [prevLines, cursorLine, nextLines].filter(Boolean).join('\n');
});

return index;

})));
