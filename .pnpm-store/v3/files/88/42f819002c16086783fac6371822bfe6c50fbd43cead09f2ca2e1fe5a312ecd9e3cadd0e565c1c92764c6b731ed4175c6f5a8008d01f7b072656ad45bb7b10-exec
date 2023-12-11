# fs-writefile-promise [![version][npm-version]][npm-url] [![License][npm-license]][license-url]

[Promise] version of [fs.writefile]:

> Asynchronously writes data to a file, replacing the file if it already exists.

[![Build Status][travis-image]][travis-url]
[![Downloads][npm-downloads]][npm-url]
[![Code Climate][codeclimate-quality]][codeclimate-url]
[![Coverage Status][codeclimate-coverage]][codeclimate-url]
[![Dependencies][david-image]][david-url]

## Install

```sh
npm install --save fs-writefile-promise
```

## API

```js
var write = require('fs-writefile-promise')
```

### write(filename, data [, options])

*filename*: `String`  
*data* `String` or `Buffer`  
*options*: `Object`  
Return: `Object` ([Promise])

When it finishes, it will be [*fulfilled*](http://promisesaplus.com/#point-26) with the file name that was written to.

When it fails, it will be [*rejected*](http://promisesaplus.com/#point-30) with an error as its first argument.

```js
write('/tmp/foo', 'bar')
  .then(function (filename) {
    console.log(filename) //=> '/tmp/foo'
  })

  .catch(function (err) {
    console.error(err)
  })
})
```

#### options

The option object will be directly passed to [fs.writefile](https://nodejs.org/api/fs.html#fs_fs_writefile_filename_data_options_callback).

## License

[ISC License](LICENSE) &copy; [Ahmad Nassri](https://www.ahmadnassri.com/)

[license-url]: https://github.com/ahmadnassri/fs-writefile-promise/blob/master/LICENSE

[travis-url]: https://travis-ci.org/ahmadnassri/fs-writefile-promise
[travis-image]: https://img.shields.io/travis/ahmadnassri/fs-writefile-promise.svg?style=flat-square

[npm-url]: https://www.npmjs.com/package/fs-writefile-promise
[npm-license]: https://img.shields.io/npm/l/fs-writefile-promise.svg?style=flat-square
[npm-version]: https://img.shields.io/npm/v/fs-writefile-promise.svg?style=flat-square
[npm-downloads]: https://img.shields.io/npm/dm/fs-writefile-promise.svg?style=flat-square

[codeclimate-url]: https://codeclimate.com/github/ahmadnassri/fs-writefile-promise
[codeclimate-quality]: https://img.shields.io/codeclimate/github/ahmadnassri/fs-writefile-promise.svg?style=flat-square
[codeclimate-coverage]: https://img.shields.io/codeclimate/coverage/github/ahmadnassri/fs-writefile-promise.svg?style=flat-square

[david-url]: https://david-dm.org/ahmadnassri/fs-writefile-promise
[david-image]: https://img.shields.io/david/ahmadnassri/fs-writefile-promise.svg?style=flat-square

[fs.writefile]: https://nodejs.org/api/fs.html#fs_fs_writefile_filename_data_options_callback
[Promise]: http://promisesaplus.com/
