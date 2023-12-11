"use strict";

require("core-js/modules/es.array.slice");

require("core-js/modules/es.string.split");

exports.__esModule = true;
exports.getPointers = void 0;

// TODO: Better error handling
var getPointers = function getPointers(dataPath) {
  var pointers = dataPath.split('/').slice(1);

  for (var index in pointers) {
    pointers[index] = pointers[index].split('~1').join('/').split('~0').join('~');
  }

  return pointers;
};

exports.getPointers = getPointers;