"use strict";

exports.__esModule = true;
exports.getPointers = void 0;

// TODO: Better error handling
const getPointers = dataPath => {
  const pointers = dataPath.split('/').slice(1);

  for (const index in pointers) {
    pointers[index] = pointers[index].split('~1').join('/').split('~0').join('~');
  }

  return pointers;
};

exports.getPointers = getPointers;