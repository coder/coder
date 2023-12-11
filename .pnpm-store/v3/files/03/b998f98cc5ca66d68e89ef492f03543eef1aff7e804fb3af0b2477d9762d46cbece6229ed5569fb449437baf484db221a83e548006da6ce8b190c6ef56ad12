"use strict";

require("core-js/modules/es.array.filter");

exports.__esModule = true;
exports.default = getMetaFromPath;

var _utils = require("./utils");

function getMetaFromPath(jsonAst, dataPath, isIdentifierLocation) {
  var pointers = (0, _utils.getPointers)(dataPath);
  var lastPointerIndex = pointers.length - 1;
  return pointers.reduce(function (obj, pointer, idx) {
    switch (obj.type) {
      case 'Object':
        {
          var filtered = obj.children.filter(function (child) {
            return child.key.value === pointer;
          });

          if (filtered.length !== 1) {
            throw new Error(`Couldn't find property ${pointer} of ${dataPath}`);
          }

          var _filtered$ = filtered[0],
              key = _filtered$.key,
              value = _filtered$.value;
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