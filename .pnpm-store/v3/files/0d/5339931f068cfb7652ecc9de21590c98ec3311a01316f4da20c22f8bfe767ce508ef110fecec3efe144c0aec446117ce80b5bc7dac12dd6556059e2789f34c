"use strict";

var _interopRequireDefault = require("@babel/runtime/helpers/interopRequireDefault");

require("core-js/modules/es.array.concat");

require("core-js/modules/es.array.map");

require("core-js/modules/es.array.sort");

require("core-js/modules/es.object.assign");

require("core-js/modules/es.object.to-string");

require("core-js/modules/es.regexp.to-string");

exports.__esModule = true;
exports.default = void 0;

var _inheritsLoose2 = _interopRequireDefault(require("@babel/runtime/helpers/inheritsLoose"));

var _chalk = _interopRequireDefault(require("chalk"));

var _leven = _interopRequireDefault(require("leven"));

var _jsonpointer = _interopRequireDefault(require("jsonpointer"));

var _base = _interopRequireDefault(require("./base"));

var EnumValidationError =
/*#__PURE__*/
function (_BaseValidationError) {
  (0, _inheritsLoose2.default)(EnumValidationError, _BaseValidationError);

  function EnumValidationError() {
    return _BaseValidationError.apply(this, arguments) || this;
  }

  var _proto = EnumValidationError.prototype;

  _proto.print = function print() {
    var _this$options = this.options,
        message = _this$options.message,
        allowedValues = _this$options.params.allowedValues;
    var bestMatch = this.findBestMatch();
    var output = [_chalk.default`{red {bold ENUM} ${message}}`, _chalk.default`{red (${allowedValues.join(', ')})}\n`];
    return output.concat(this.getCodeFrame(bestMatch !== null ? _chalk.default`👈🏽  Did you mean {magentaBright ${bestMatch}} here?` : _chalk.default`👈🏽  Unexpected value, should be equal to one of the allowed values`));
  };

  _proto.getError = function getError() {
    var _this$options2 = this.options,
        message = _this$options2.message,
        dataPath = _this$options2.dataPath,
        params = _this$options2.params;
    var bestMatch = this.findBestMatch();
    var output = Object.assign({}, this.getLocation(), {
      error: `${this.getDecoratedPath(dataPath)} ${message}: ${params.allowedValues.join(', ')}`,
      path: dataPath
    });

    if (bestMatch !== null) {
      output.suggestion = `Did you mean ${bestMatch}?`;
    }

    return output;
  };

  _proto.findBestMatch = function findBestMatch() {
    var _this$options3 = this.options,
        dataPath = _this$options3.dataPath,
        allowedValues = _this$options3.params.allowedValues;
    var currentValue = dataPath === '' ? this.data : _jsonpointer.default.get(this.data, dataPath);

    if (!currentValue) {
      return null;
    }

    var bestMatch = allowedValues.map(function (value) {
      return {
        value,
        weight: (0, _leven.default)(value, currentValue.toString())
      };
    }).sort(function (x, y) {
      return x.weight > y.weight ? 1 : x.weight < y.weight ? -1 : 0;
    })[0];
    return allowedValues.length === 1 || bestMatch.weight < bestMatch.value.length ? bestMatch.value : null;
  };

  return EnumValidationError;
}(_base.default);

exports.default = EnumValidationError;
module.exports = exports.default;