"use strict";

require("core-js/modules/es.array.filter");

exports.__esModule = true;
exports.default = getDecoratedDataPath;

var _utils = require("./utils");

function getDecoratedDataPath(jsonAst, dataPath) {
  var decoratedPath = '';
  (0, _utils.getPointers)(dataPath).reduce(function (obj, pointer) {
    switch (obj.type) {
      case 'Object':
        {
          decoratedPath += `/${pointer}`;
          var filtered = obj.children.filter(function (child) {
            return child.key.value === pointer;
          });

          if (filtered.length !== 1) {
            throw new Error(`Couldn't find property ${pointer} of ${dataPath}`);
          }

          return filtered[0].value;
        }

      case 'Array':
        decoratedPath += `/${pointer}${getTypeName(obj.children[pointer])}`;
        return obj.children[pointer];

      default:
        // eslint-disable-next-line no-console
        console.log(obj);
    }
  }, jsonAst);
  return decoratedPath;
}

function getTypeName(obj) {
  if (!obj || !obj.children) {
    return '';
  }

  var type = obj.children.filter(function (child) {
    child && child.key && child.key.value === 'type';
  });

  if (!type.length) {
    return '';
  }

  return type[0].value && `:${type[0].value.value}` || '';
}

module.exports = exports.default;