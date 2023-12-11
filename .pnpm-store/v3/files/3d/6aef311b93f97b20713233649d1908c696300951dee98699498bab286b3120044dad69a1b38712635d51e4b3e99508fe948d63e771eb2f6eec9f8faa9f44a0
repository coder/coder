"use strict";

exports.__esModule = true;
exports.default = void 0;

var _jsonToAst = _interopRequireDefault(require("json-to-ast"));

var _helpers = _interopRequireDefault(require("./helpers"));

function _interopRequireDefault(obj) { return obj && obj.__esModule ? obj : { default: obj }; }

var _default = (schema, data, errors, options = {}) => {
  const {
    format = 'cli',
    indent = null
  } = options;
  const jsonRaw = JSON.stringify(data, null, indent);
  const jsonAst = (0, _jsonToAst.default)(jsonRaw, {
    loc: true
  });

  const customErrorToText = error => error.print().join('\n');

  const customErrorToStructure = error => error.getError();

  const customErrors = (0, _helpers.default)(errors, {
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