'use strict';

var fs = require('graceful-fs');

module.exports = function fsReadFilePromise(filePath, options) {
  var resolve;
  var reject;

  fs.readFile(filePath, options, function(err, buf) {
    if (err) {
      reject(err);
      return;
    }
    resolve(buf);
  });

  return new Promise(function(_resolve, _reject) {
    resolve = _resolve;
    reject = _reject;
  });
};
