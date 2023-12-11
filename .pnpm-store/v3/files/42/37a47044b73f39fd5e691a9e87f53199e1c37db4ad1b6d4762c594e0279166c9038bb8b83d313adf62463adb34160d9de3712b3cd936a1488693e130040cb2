"use strict";

var _interopRequireDefault = require("@babel/runtime/helpers/interopRequireDefault");

require("core-js/modules/es.array.concat");

require("core-js/modules/es.object.assign");

exports.__esModule = true;
exports.default = void 0;

var _inheritsLoose2 = _interopRequireDefault(require("@babel/runtime/helpers/inheritsLoose"));

var _chalk = _interopRequireDefault(require("chalk"));

var _base = _interopRequireDefault(require("./base"));

var RequiredValidationError =
/*#__PURE__*/
function (_BaseValidationError) {
  (0, _inheritsLoose2.default)(RequiredValidationError, _BaseValidationError);

  function RequiredValidationError() {
    return _BaseValidationError.apply(this, arguments) || this;
  }

  var _proto = RequiredValidationError.prototype;

  _proto.getLocation = function getLocation(dataPath) {
    if (dataPath === void 0) {
      dataPath = this.options.dataPath;
    }

    var _BaseValidationError$ = _BaseValidationError.prototype.getLocation.call(this, dataPath),
        start = _BaseValidationError$.start;

    return {
      start
    };
  };

  _proto.print = function print() {
    var _this$options = this.options,
        message = _this$options.message,
        params = _this$options.params;
    var output = [_chalk.default`{red {bold REQUIRED} ${message}}\n`];
    return output.concat(this.getCodeFrame(_chalk.default`☹️  {magentaBright ${params.missingProperty}} is missing here!`));
  };

  _proto.getError = function getError() {
    var _this$options2 = this.options,
        message = _this$options2.message,
        dataPath = _this$options2.dataPath;
    return Object.assign({}, this.getLocation(), {
      error: `${this.getDecoratedPath(dataPath)} ${message}`,
      path: dataPath
    });
  };

  return RequiredValidationError;
}(_base.default);

exports.default = RequiredValidationError;
module.exports = exports.default;