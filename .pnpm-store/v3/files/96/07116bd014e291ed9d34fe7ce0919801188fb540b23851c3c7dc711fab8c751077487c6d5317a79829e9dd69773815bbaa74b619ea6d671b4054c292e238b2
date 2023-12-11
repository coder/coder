"use strict";

exports.__esModule = true;
exports.default = void 0;

var _codeFrame = require("@babel/code-frame");

var _json = require("../json");

class BaseValidationError {
  constructor(options = {
    isIdentifierLocation: false
  }, {
    data,
    schema,
    jsonAst,
    jsonRaw
  }) {
    this.options = options;
    this.data = data;
    this.schema = schema;
    this.jsonAst = jsonAst;
    this.jsonRaw = jsonRaw;
  }

  getLocation(dataPath = this.options.dataPath) {
    const {
      isIdentifierLocation,
      isSkipEndLocation
    } = this.options;
    const {
      loc
    } = (0, _json.getMetaFromPath)(this.jsonAst, dataPath, isIdentifierLocation);
    return {
      start: loc.start,
      end: isSkipEndLocation ? undefined : loc.end
    };
  }

  getDecoratedPath(dataPath = this.options.dataPath) {
    const decoratedPath = (0, _json.getDecoratedDataPath)(this.jsonAst, dataPath);
    return decoratedPath;
  }

  getCodeFrame(message, dataPath = this.options.dataPath) {
    return (0, _codeFrame.codeFrameColumns)(this.jsonRaw, this.getLocation(dataPath), {
      highlightCode: true,
      message
    });
  }

  print() {
    throw new Error(`Implement the 'print' method inside ${this.constructor.name}!`);
  }

  getError() {
    throw new Error(`Implement the 'getError' method inside ${this.constructor.name}!`);
  }

}

exports.default = BaseValidationError;
module.exports = exports.default;