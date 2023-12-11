"use strict";

require("core-js/modules/es.array.iterator");

exports.__esModule = true;
exports.default = void 0;

var _chalk = _interopRequireDefault(require("chalk"));

var _base = _interopRequireDefault(require("./base"));

function _interopRequireDefault(obj) { return obj && obj.__esModule ? obj : { default: obj }; }

class AdditionalPropValidationError extends _base.default {
  constructor(...args) {
    super(...args);
    this.options.isIdentifierLocation = true;
  }

  print() {
    const {
      message,
      dataPath,
      params
    } = this.options;
    const output = [_chalk.default`{red {bold ADDTIONAL PROPERTY} ${message}}\n`];
    return output.concat(this.getCodeFrame(_chalk.default`😲  {magentaBright ${params.additionalProperty}} is not expected to be here!`, `${dataPath}/${params.additionalProperty}`));
  }

  getError() {
    const {
      params,
      dataPath
    } = this.options;
    return Object.assign({}, this.getLocation(`${dataPath}/${params.additionalProperty}`), {
      error: `${this.getDecoratedPath(dataPath)} Property ${params.additionalProperty} is not expected to be here`,
      path: dataPath
    });
  }

}

exports.default = AdditionalPropValidationError;
module.exports = exports.default;