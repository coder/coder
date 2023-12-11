"use strict";

exports.__esModule = true;
exports.default = getMetaFromPath;

var _utils = require("./utils");

function getMetaFromPath(jsonAst, dataPath, isIdentifierLocation) {
  const pointers = (0, _utils.getPointers)(dataPath);
  const lastPointerIndex = pointers.length - 1;
  return pointers.reduce((obj, pointer, idx) => {
    switch (obj.type) {
      case 'Object':
        {
          const filtered = obj.children.filter(child => child.key.value === pointer);

          if (filtered.length !== 1) {
            throw new Error(`Couldn't find property ${pointer} of ${dataPath}`);
          }

          const {
            key,
            value
          } = filtered[0];
          return isIdentifierLocation && idx === lastPointerIndex ? key : value;
        }

      case 'Array':
        return obj.children[pointer];

      default:
        // eslint-disable-next-line no-console
        console.log(obj);
    }
  }, jsonAst);
}

module.exports = exports.default;