"use strict";

var _interopRequireDefault = require("@babel/runtime/helpers/interopRequireDefault");

require("core-js/modules/es.array.map");

exports.__esModule = true;
exports.default = void 0;

var _jsonToAst = _interopRequireDefault(require("json-to-ast"));

var _helpers = _interopRequireDefault(require("./helpers"));

var _default = function _default(schema, data, errors, options) {
  if (options === void 0) {
    options = {};
  }

  var _options = options,
      _options$format = _options.format,
      format = _options$format === void 0 ? 'cli' : _options$format,
      _options$indent = _options.indent,
      indent = _options$indent === void 0 ? null : _options$indent;
  var jsonRaw = JSON.stringify(data, null, indent);
  var jsonAst = (0, _jsonToAst.default)(jsonRaw, {
    loc: true
  });

  var customErrorToText = function customErrorToText(error) {
    return error.print().join('\n');
  };

  var customErrorToStructure = function customErrorToStructure(error) {
    return error.getError();
  };

  var customErrors = (0, _helpers.default)(errors, {
    data,
    schema,
    jsonAst,
    jsonRaw
  });

  if (format === 'cli') {
    return customErrors.map(customErrorToText).join('\n\n');
  } else {
    return customErrors.map(customErrorToStructure);
  }
};

exports.default = _default;
module.exports = exports.default;