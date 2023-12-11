# fs-readfile-promise 

[![NPM version](https://img.shields.io/npm/v/fs-readfile-promise.svg)](https://www.npmjs.com/package/fs-readfile-promise)
[![Build Status](https://travis-ci.org/shinnn/fs-readfile-promise.svg?branch=master)](https://travis-ci.org/shinnn/fs-readfile-promise)
[![Build status](https://ci.appveyor.com/api/projects/status/5sacvq0w9x7mwkwd?svg=true)](https://ci.appveyor.com/project/ShinnosukeWatanabe/fs-readfile-promise)
[![Coverage Status](https://img.shields.io/coveralls/shinnn/fs-readfile-promise.svg)](https://coveralls.io/r/shinnn/fs-readfile-promise)
[![Dependency Status](https://img.shields.io/david/shinnn/fs-readfile-promise.svg?label=deps)](https://david-dm.org/shinnn/fs-readfile-promise)
[![devDependency Status](https://img.shields.io/david/dev/shinnn/fs-readfile-promise.svg?label=devDeps)](https://david-dm.org/shinnn/fs-readfile-promise#info=devDependencies)

[Promises/A+][promise] version of [`fs.readFile`][fs.readfile]

```javascript
var readFile = require('fs-readfile-promise');

readFile('path/to/file')
.then(buffer => console.log(buffer.toString()))
.catch(err => console.log(err.message));
```

Based on the principle of [*modular programming*](https://en.wikipedia.org/wiki/Modular_programming), this module has only one functionality [`readFile`][fs.readfile], unlike other promise-based file system modules. If you want to use a bunch of other [`fs`](http://nodejs.org/api/fs.html) methods in the promises' way, choose other modules such as [q-io](https://github.com/kriskowal/q-io) and [fs-promise](https://github.com/kevinbeaty/fs-promise).

## Installation

[Use npm.](https://docs.npmjs.com/cli/install)

```
npm install fs-readfile-promise
```

## API

```javascript
const readFile = require('fs-readfile-promise');
```

### readFile(*filename* [, *options*])

*filename*: `String`  
*options*: `Object` or `String` ([fs.readFile] options)  
Return: `Object` ([Promise][promise])

When it finish reading the file, it will be [*fulfilled*](https://promisesaplus.com/#point-26) with an [`Buffer`](https://nodejs.org/api/buffer.html#buffer_buffer) of the file as its first argument.

When it fails to read the file, it will be [*rejected*](https://promisesaplus.com/#point-30) with an error as its first argument.

```javascript
const readFile = require('fs-readfile-promise');

const onFulfilled = buffer => console.log(buffer.toString());
const onRejected = err => console.log('Cannot read the file.');

readFile('path/to/file').then(onFulfilled, onRejected);
```

## License

Copyright (c) 2014 - 2015 [Shinnosuke Watanabe](https://github.com/shinnn)

Licensed under [the MIT License](./LICENSE).

[fs.readfile]: https://nodejs.org/api/fs.html#fs_fs_readfile_filename_options_callback
[promise]: https://promisesaplus.com/
