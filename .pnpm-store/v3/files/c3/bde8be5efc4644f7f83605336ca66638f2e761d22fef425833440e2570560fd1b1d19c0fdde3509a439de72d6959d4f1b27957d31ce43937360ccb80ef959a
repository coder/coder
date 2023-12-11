# doT

Created in search of the fastest and concise JavaScript templating function with emphasis on performance under V8 and nodejs. It shows great performance for both nodejs and browsers.

doT.js is fast, small and has no dependencies.

[![Build Status](https://travis-ci.org/olado/doT.svg?branch=master)](https://travis-ci.org/olado/doT)
[![npm version](https://badge.fury.io/js/dot.svg)](https://www.npmjs.com/package/dot)
[![Coverage Status](http://coveralls.io/repos/github/olado/doT/badge.svg?branch=master)](https://coveralls.io/github/olado/doT?branch=master)


## Note from the maintainer

doT is a really solid piece of software engineering (I didn’t create it) that is rarely updated exactly for this reason.

It took me years to grasp how it works even though it’s only 140 lines of code - it looks like magic.

I used it in my other projects (e.g. [ajv](https://github.com/epoberezkin/ajv)) as the smallest, the fastest and the most functional (all three!) templating engine ever made, that is particularly useful in all code generation scenarios where manipulating AST is an overkill.

It’s a race car of templating engines - doT lacks bells and whistles that other templating engines have, but it allows to achive more than any other, if you use it right (YMMV).


## Features
    custom delimiters
    runtime evaluation
    runtime interpolation
    compile-time evaluation
    partials support
    conditionals support
    array iterators
    encoding
    control whitespace - strip or preserve
    streaming friendly
    use it as logic-less or with logic, it is up to you

## Docs, live playground and samples

http://olado.github.com/doT (todo: update docs with new features added in version 1.0.0)

## New in version 1.0.0

#### Added parameters support in partials

```html
{{##def.macro:param:
	<div>{{=param.foo}}</div>
#}}

{{#def.macro:myvariable}}
```

#### Node module now supports auto-compilation of dot templates from specified path

```js
var dots = require("dot").process({ path: "./views"});
```

This will compile .def, .dot, .jst files found under the specified path.
Details
   * It ignores sub-directories.
   * Template files can have multiple extensions at the same time.
   * Files with .def extension can be included in other files via {{#def.name}}
   * Files with .dot extension are compiled into functions with the same name and
   can be accessed as renderer.filename
   * Files with .jst extension are compiled into .js files. Produced .js file can be
   loaded as a commonJS, AMD module, or just installed into a global variable (default is set to window.render)
   * All inline defines defined in the .jst file are
   compiled into separate functions and are available via _render.filename.definename
 
   Basic usage:
 ```js
        var dots = require("dot").process({path: "./views"});
        dots.mytemplate({foo:"hello world"});
 ```
   The above snippet will:
	* Compile all templates in views folder (.dot, .def, .jst)
  	* Place .js files compiled from .jst templates into the same folder
     	   These files can be used with require, i.e. require("./views/mytemplate")
  	* Return an object with functions compiled from .dot templates as its properties
  	* Render mytemplate template
 
#### CLI tool to compile dot templates into js files

	./bin/dot-packer -s examples/views -d out/views

## Example for express
	Many people are using doT with express. I added an example of the best way of doing it examples/express:

[doT with express](examples/express)

## Notes
    doU.js is here only so that legacy external tests do not break. Use doT.js.
    doT.js with doT.templateSettings.append=false provides the same performance as doU.js.

## Security considerations

doT allows arbitrary JavaScript code in templates, making it one of the most flexible and powerful templating engines. It means that doT security model assumes that you only use trusted templates and you don't use any  user input as any part of the template, as otherwise it can lead to code injection.

It is strongly recommended to compile all templates to JS code as early as possible. Possible options:

- using doT as dev-dependency only and compiling templates to JS files, for example, as described above or using a custom script, during the build. This is the most performant and secure approach and it is strongly recommended.
- if the above approach is not possible for some reason (e.g. templates are dynamically generated using some run-time data), it is recommended to compile templates to in-memory functions during application start phase, before any external input is processed.
- compiling templates lazily, on demand, is less safe. Even though the possibility of the code injection via prototype pollution was patched (#291), there may be some other unknown vulnerabilities that could lead to code injection.

Please report any found vulnerabilities to npm, not via issue tracker.

## Author
Laura Doktorova [@olado](http://twitter.com/olado)

## License
doT is licensed under the MIT License. (See LICENSE-DOT)

<p align="center">
  <img src="http://olado.github.io/doT/doT-js-100@2x.png" alt="logo by Kevin Kirchner"/>
</p>

Thank you [@KevinKirchner](https://twitter.com/kevinkirchner) for the logo.
