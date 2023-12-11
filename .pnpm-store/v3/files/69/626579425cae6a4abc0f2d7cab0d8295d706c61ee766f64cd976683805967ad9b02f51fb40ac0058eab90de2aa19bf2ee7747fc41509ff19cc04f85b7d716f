# Converting an OpenAPI/Swagger file to Markdown with the Widdershins JavaScript interface

Using Widdershins in a JavaScript program gives you control over the full range of options.
To use Widdershins from the CLI, see [Converting an OpenAPI/Swagger file to Markdown with the Widdershins CLI](ConvertingFilesBasicCLI.md).

## Prerequisites

- Install NodeJS and Node Package Manager (NPM).
See [nodejs.org](https://nodejs.org/).
- If you don't already have an NPM project, run `npm init` from the folder in which you want to create the program.
NPM walks you through the process of setting up an NPM project and creates a `package.json` file to store the project configuration.
Most of the NPM settings are not relevant to Widdershins; the important part of the process is that it sets up a project that can install and manage NPM packages such as Widdershins so you can use those packages in your programs.
- From the root folder of your project (the folder that contains the `package.json` file), add Widdershins as a dependency by running this command:
```shell
npm install --save widdershins
```

Now you can use Widdershins in JavaScript programs in the project.

## Converting files with JavaScript

1. Create a JavaScript program with the following general steps.
You can name the file anything you want.
1. In the JavaScript file, import Widdershins so you can use it in the program:
```javascript
const widdershins = require('widdershins');
```
1. Set up your options in an `options` object.
Use the JavaScript parameter name from the [README.md](https://github.com/Mermade/widdershins#options) file, not the CLI parameter name.
For example, these options generate code samples in Python and Ruby:
```javascript
const options = {
  language_tabs: [{ python: "Python" }, { ruby: "Ruby" }]
};
```
1. Import and parse the OpenAPI or Swagger file.
This example uses the NodeJS FileSystem and JSON packages:
```javascript
const fs = require('fs');
const fileData = fs.readFileSync('swagger.json', 'utf8');
const swaggerFile = JSON.parse(fileData);
```
1. Use Widdershins to convert the file.
Widdershins returns the converted Markdown via a Promise:
```javascript
widdershins.convert(swaggerFile, options)
.then(markdownOutput => {
  // markdownOutput contains the converted markdown
})
.catch(err => {
  // handle errors
});
```
1. When the Promise resolves, write the Markdown to a file:
```javascript
widdershins.convert(swaggerFile, options)
.then(markdownOutput => {
  // markdownOutput contains the converted markdown
  fs.writeFileSync('myOutput.md', markdownOutput, 'utf8');
})
.catch(err => {
  // handle errors
});
```
1. Run the JavaScript program:
```shell
node convertMarkdown.js
```

The complete JavaScript program looks like this:

```javascript
const widdershins = require('widdershins');
const fs = require('fs');

const options = {
  language_tabs: [{ python: "Python" }, { ruby: "Ruby" }]
};

const fileData = fs.readFileSync('swagger.json', 'utf8');
const swaggerFile = JSON.parse(fileData);

widdershins.convert(swaggerFile, options)
.then(markdownOutput => {
  // markdownOutput contains the converted markdown
  fs.writeFileSync('myOutput.md', markdownOutput, 'utf8');
})
.catch(err => {
  // handle errors
});
```

Now you can use the Markdown file in your documentation or use a tool such as [Shins](https://github.com/Mermade/shins) to convert it to HTML.
