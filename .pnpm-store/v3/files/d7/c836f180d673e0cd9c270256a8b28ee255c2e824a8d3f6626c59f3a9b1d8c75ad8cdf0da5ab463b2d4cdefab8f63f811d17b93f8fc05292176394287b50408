"use strict";

exports.__esModule = true;
exports.default = void 0;

var _chalk = _interopRequireDefault(require("chalk"));

var _base = _interopRequireDefault(require("./base"));

function _interopRequireDefault(obj) { return obj && obj.__esModule ? obj : { default: obj }; }

class DefaultValidationError extends _base.default {
  print() {
    const {
      keyword,
      message
    } = this.options;
    const output = [_chalk.default`{red {bold ${keyword.toUpperCase()}} ${message}}\n`];
    return output.concat(this.getCodeFrame(_chalk.default`👈🏽  {magentaBright ${keyword}} ${message}`));
  }

  getError() {
    const {
      keyword,
      message,
      dataPath
    } = this.options;
    return Object.assign({}, this.getLocation(), {
      error: `${this.getDecoratedPath(dataPath)}: ${keyword} ${message}`,
      path: dataPath
    });
  }

}

exports.default = DefaultValidationError;
module.exports = exports.default;