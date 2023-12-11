"use strict";

var _interopRequireDefault = require("@babel/runtime/helpers/interopRequireDefault");

require("core-js/modules/es.array.concat");

require("core-js/modules/es.object.assign");

exports.__esModule = true;
exports.default = void 0;

var _inheritsLoose2 = _interopRequireDefault(require("@babel/runtime/helpers/inheritsLoose"));

var _chalk = _interopRequireDefault(require("chalk"));

var _base = _interopRequireDefault(require("./base"));

var DefaultValidationError =
/*#__PURE__*/
function (_BaseValidationError) {
  (0, _inheritsLoose2.default)(DefaultValidationError, _BaseValidationError);

  function DefaultValidationError() {
    return _BaseValidationError.apply(this, arguments) || this;
  }

  var _proto = DefaultValidationError.prototype;

  _proto.print = function print() {
    var _this$options = this.options,
        keyword = _this$options.keyword,
        message = _this$options.message;
    var output = [_chalk.default`{red {bold ${keyword.toUpperCase()}} ${message}}\n`];
    return output.concat(this.getCodeFrame(_chalk.default`👈🏽  {magentaBright ${keyword}} ${message}`));
  };

  _proto.getError = function getError() {
    var _this$options2 = this.options,
        keyword = _this$options2.keyword,
        message = _this$options2.message,
        dataPath = _this$options2.dataPath;
    return Object.assign({}, this.getLocation(), {
      error: `${this.getDecoratedPath(dataPath)}: ${keyword} ${message}`,
      path: dataPath
    });
  };

  return DefaultValidationError;
}(_base.default);

exports.default = DefaultValidationError;
module.exports = exports.default;