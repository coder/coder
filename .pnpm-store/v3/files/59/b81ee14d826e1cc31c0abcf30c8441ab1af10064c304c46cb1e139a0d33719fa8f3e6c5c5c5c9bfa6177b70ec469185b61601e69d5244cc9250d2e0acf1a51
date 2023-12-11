"use strict";

var _interopRequireDefault = require("@babel/runtime/helpers/interopRequireDefault");

require("core-js/modules/es.array.concat");

require("core-js/modules/es.object.assign");

exports.__esModule = true;
exports.default = void 0;

var _inheritsLoose2 = _interopRequireDefault(require("@babel/runtime/helpers/inheritsLoose"));

var _chalk = _interopRequireDefault(require("chalk"));

var _base = _interopRequireDefault(require("./base"));

var AdditionalPropValidationError =
/*#__PURE__*/
function (_BaseValidationError) {
  (0, _inheritsLoose2.default)(AdditionalPropValidationError, _BaseValidationError);

  function AdditionalPropValidationError() {
    var _this;

    for (var _len = arguments.length, args = new Array(_len), _key = 0; _key < _len; _key++) {
      args[_key] = arguments[_key];
    }

    _this = _BaseValidationError.call.apply(_BaseValidationError, [this].concat(args)) || this;
    _this.options.isIdentifierLocation = true;
    return _this;
  }

  var _proto = AdditionalPropValidationError.prototype;

  _proto.print = function print() {
    var _this$options = this.options,
        message = _this$options.message,
        dataPath = _this$options.dataPath,
        params = _this$options.params;
    var output = [_chalk.default`{red {bold ADDTIONAL PROPERTY} ${message}}\n`];
    return output.concat(this.getCodeFrame(_chalk.default`😲  {magentaBright ${params.additionalProperty}} is not expected to be here!`, `${dataPath}/${params.additionalProperty}`));
  };

  _proto.getError = function getError() {
    var _this$options2 = this.options,
        params = _this$options2.params,
        dataPath = _this$options2.dataPath;
    return Object.assign({}, this.getLocation(`${dataPath}/${params.additionalProperty}`), {
      error: `${this.getDecoratedPath(dataPath)} Property ${params.additionalProperty} is not expected to be here`,
      path: dataPath
    });
  };

  return AdditionalPropValidationError;
}(_base.default);

exports.default = AdditionalPropValidationError;
module.exports = exports.default;