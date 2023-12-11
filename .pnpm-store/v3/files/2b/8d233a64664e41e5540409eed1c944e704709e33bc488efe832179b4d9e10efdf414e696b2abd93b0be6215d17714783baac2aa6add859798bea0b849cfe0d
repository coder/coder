'use strict';

var ver = process.versions.node;
var majorVer = parseInt(ver.split('.')[0], 10);

if (majorVer < 4) {
  // eslint-disable-next-line no-console
  console.error(
    'Node version ' +
      ver +
      ' is not supported, please use Node.js 4.0 or higher.'
  );
  process.exit(1);
} else if (majorVer < 8) {
  module.exports = require('./lib/legacy');
} else {
  module.exports = require('./lib/modern');
}
