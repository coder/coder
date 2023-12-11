"use strict";

exports.__esModule = true;
exports.default = void 0;

var _chalk = _interopRequireDefault(require("chalk"));

var _base = _interopRequireDefault(require("./base"));

function _interopRequireDefault(obj) { return obj && obj.__esModule ? obj : { default: obj }; }

class RequiredValidationError extends _base.default {
  getLocation(dataPath = this.options.dataPath) {
    const {
      start
    } = super.getLocation(dataPath);
    return {
      start
    };
  }

  print() {
    const {
      message,
      params
    } = this.options;
    const output = [_chalk.default`{red {bold REQUIRED} ${message}}\n`];
    return output.concat(this.getCodeFrame(_chalk.default`☹️  {magentaBright ${params.missingProperty}} is missing here!`));
  }

  getError() {
    const {
      message,
      dataPath
    } = this.options;
    return Object.assign({}, this.getLocation(), {
      error: `${this.getDecoratedPath(dataPath)} ${message}`,
      path: dataPath
    });
  }

}

exports.default = RequiredValidationError;
module.exports = exports.default;