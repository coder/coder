"use strict";

exports.__esModule = true;
exports.default = void 0;

var _codeFrame = require("@babel/code-frame");

var _json = require("../json");

var BaseValidationError =
/*#__PURE__*/
function () {
  function BaseValidationError(options, _ref) {
    if (options === void 0) {
      options = {
        isIdentifierLocation: false
      };
    }

    var data = _ref.data,
        schema = _ref.schema,
        jsonAst = _ref.jsonAst,
        jsonRaw = _ref.jsonRaw;
    this.options = options;
    this.data = data;
    this.schema = schema;
    this.jsonAst = jsonAst;
    this.jsonRaw = jsonRaw;
  }

  var _proto = BaseValidationError.prototype;

  _proto.getLocation = function getLocation(dataPath) {
    if (dataPath === void 0) {
      dataPath = this.options.dataPath;
    }

    var _this$options = this.options,
        isIdentifierLocation = _this$options.isIdentifierLocation,
        isSkipEndLocation = _this$options.isSkipEndLocation;

    var _getMetaFromPath = (0, _json.getMetaFromPath)(this.jsonAst, dataPath, isIdentifierLocation),
        loc = _getMetaFromPath.loc;

    return {
      start: loc.start,
      end: isSkipEndLocation ? undefined : loc.end
    };
  };

  _proto.getDecoratedPath = function getDecoratedPath(dataPath) {
    if (dataPath === void 0) {
      dataPath = this.options.dataPath;
    }

    var decoratedPath = (0, _json.getDecoratedDataPath)(this.jsonAst, dataPath);
    return decoratedPath;
  };

  _proto.getCodeFrame = function getCodeFrame(message, dataPath) {
    if (dataPath === void 0) {
      dataPath = this.options.dataPath;
    }

    return (0, _codeFrame.codeFrameColumns)(this.jsonRaw, this.getLocation(dataPath), {
      highlightCode: true,
      message
    });
  };

  _proto.print = function print() {
    throw new Error(`Implement the 'print' method inside ${this.constructor.name}!`);
  };

  _proto.getError = function getError() {
    throw new Error(`Implement the 'getError' method inside ${this.constructor.name}!`);
  };

  return BaseValidationError;
}();

exports.default = BaseValidationError;
module.exports = exports.default;