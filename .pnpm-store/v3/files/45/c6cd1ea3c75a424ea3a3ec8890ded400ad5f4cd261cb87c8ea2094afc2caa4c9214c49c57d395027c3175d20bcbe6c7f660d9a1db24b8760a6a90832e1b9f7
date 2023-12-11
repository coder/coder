"use strict";

require("core-js/modules/es.array.sort");

exports.__esModule = true;
exports.default = void 0;

var _chalk = _interopRequireDefault(require("chalk"));

var _leven = _interopRequireDefault(require("leven"));

var _jsonpointer = _interopRequireDefault(require("jsonpointer"));

var _base = _interopRequireDefault(require("./base"));

function _interopRequireDefault(obj) { return obj && obj.__esModule ? obj : { default: obj }; }

class EnumValidationError extends _base.default {
  print() {
    const {
      message,
      params: {
        allowedValues
      }
    } = this.options;
    const bestMatch = this.findBestMatch();
    const output = [_chalk.default`{red {bold ENUM} ${message}}`, _chalk.default`{red (${allowedValues.join(', ')})}\n`];
    return output.concat(this.getCodeFrame(bestMatch !== null ? _chalk.default`👈🏽  Did you mean {magentaBright ${bestMatch}} here?` : _chalk.default`👈🏽  Unexpected value, should be equal to one of the allowed values`));
  }

  getError() {
    const {
      message,
      dataPath,
      params
    } = this.options;
    const bestMatch = this.findBestMatch();
    const output = Object.assign({}, this.getLocation(), {
      error: `${this.getDecoratedPath(dataPath)} ${message}: ${params.allowedValues.join(', ')}`,
      path: dataPath
    });

    if (bestMatch !== null) {
      output.suggestion = `Did you mean ${bestMatch}?`;
    }

    return output;
  }

  findBestMatch() {
    const {
      dataPath,
      params: {
        allowedValues
      }
    } = this.options;
    const currentValue = dataPath === '' ? this.data : _jsonpointer.default.get(this.data, dataPath);

    if (!currentValue) {
      return null;
    }

    const bestMatch = allowedValues.map(value => ({
      value,
      weight: (0, _leven.default)(value, currentValue.toString())
    })).sort((x, y) => x.weight > y.weight ? 1 : x.weight < y.weight ? -1 : 0)[0];
    return allowedValues.length === 1 || bestMatch.weight < bestMatch.value.length ? bestMatch.value : null;
  }

}

exports.default = EnumValidationError;
module.exports = exports.default;