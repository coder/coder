"use strict";

exports.__esModule = true;
exports.default = getDecoratedDataPath;

var _utils = require("./utils");

function getDecoratedDataPath(jsonAst, dataPath) {
  let decoratedPath = '';
  (0, _utils.getPointers)(dataPath).reduce((obj, pointer) => {
    switch (obj.type) {
      case 'Object':
        {
          decoratedPath += `/${pointer}`;
          const filtered = obj.children.filter(child => child.key.value === pointer);

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

  const type = obj.children.filter(child => {
    child && child.key && child.key.value === 'type';
  });

  if (!type.length) {
    return '';
  }

  return type[0].value && `:${type[0].value.value}` || '';
}

module.exports = exports.default;