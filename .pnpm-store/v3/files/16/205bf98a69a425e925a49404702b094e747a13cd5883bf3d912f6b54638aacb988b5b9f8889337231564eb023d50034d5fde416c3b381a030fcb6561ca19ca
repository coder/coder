# widdershins
OpenAPI / Swagger / AsyncAPI / Semoasa definition to [Slate](https://github.com/lord/slate) /
[Shins](https://github.com/mermade/shins) compatible markdown

![Build](https://img.shields.io/travis/Mermade/widdershins/master.svg) [![Tested on APIs.guru](https://api.apis.guru/badges/tested_on.svg)](https://APIs.guru) [![Tested on Mermade OpenAPIs](https://img.shields.io/badge/Additional%20Specs-419-brightgreen.svg)](https://github.com/mermade/OpenAPI_specifications)
[![Known Vulnerabilities](https://snyk.io/test/npm/widdershins/badge.svg)](https://snyk.io/test/npm/widdershins)

<img src="http://mermade.github.io/widdershins/logo.png" width="247px" height="250px" />

### Widdershins *adverb*:
* In a direction contrary to the sun's course;
* anticlockwise;
* helping you produce static documentation from your OpenAPI 3.0 / Swagger 2.0 / AsyncAPI 1.x / Semoasa 0.1.0 definition

![Widdershins screenshot](https://mermade.github.io/widdershins/screenshot.png)

### News

* Version 4.0 changes:
  * Now uses Promises not callbacks
  * Option to output html directly, and to ReSpec format
  * Unified JavaScript and Node.js code-samples, PHP added
  * `restrictions` column (`readOnly`/`writeOnly`) added to schema templates
  * Numerous bug fixes
* As of v3.0.0 Widdershins no longer expands the definition of OpenAPI body parameters / requestBodies by default, unless they have an inline schema. You can restore the old behaviour by using the `--expandBody` option.
* You may limit the depth of schema examples using the `--maxDepth` option. The default is 10.
* To omit schemas entirely, please copy and customise the `main.dot` template.
* As of v3.1.0 Widdershins includes a generated `Authorization` header in OpenAPI code samples. If you wish to omit this, see [here](/templates/openapi3/README.md).

### To install

* Clone the git repository, and `npm i` to install dependencies, or
* `npm install [-g] widdershins` to install globally

### Getting started

Widdershins is generally used as a stage in an API documentation pipeline. The pipeline begins with an API definition in OpenAPI 3.x, OpenAPI 2.0 (fka Swagger), API Blueprint, AsyncAPI or Semoasa format. Widdershins converts this description into markdown suitable for use by a **renderer**, such as [Slate](https://github.com/lord/slate), [Shins](https://github.com/mermade/shins) or html suitable for use with [ReSpec](https://github.com/w3c/respec).

If you need to create your input API definition, [this list of available editors](https://apis.guru/awesome-openapi3/category.html#editors) may be useful.

More in-depth documentation is [available here](https://mermade.github.io/widdershins).

### Examples

```
node widdershins --search false --language_tabs 'ruby:Ruby' 'python:Python' --summary defs/petstore3.json -o petstore3.md
```

### Options

| CLI parameter name | JavaScript parameter name | Type | Default value | Description |
| --- | --- | --- | --- | --- |
| --customApiKeyValue | options.customApiKeyValue | `string` | `ApiKey` | Set a custom API key value to use as the API key in generated code examples. |
| --expandBody | options.expandBody | `boolean` | `false` | If a method's requestBody parameter refers to a schema by reference (not with a inline schema), by default, Widdershins shows only a reference to this parameter. Set this option to true to expand the schema and show all properties in the request body. |
| --headings | options.headings | `integer` | 2 | Set the value of the `headingLevel` parameter in the header so Shins knows how many heading levels to show in the table of contents. Currently supported only by Shins, not by Slate, which lacks this feature. |
| --omitBody | options.omitBody | `boolean` | `false` | By default, Widdershins includes the body parameter as a row in the parameters table before the rows that represent the fields in the body. Set this parameter to omit that body parameter row. |
| --omitHeader | options.omitHeader | `boolean` | `false` | Omit the header / YAML front-matter in the generated Markdown file. |
| --resolve | options.resolve | `boolean` | `false` | Resolve external $refs, using the `source` parameter or the input file as the base location. |
| --shallowSchemas | options.shallowSchemas | `boolean` | `false` | When referring to a schema with a $ref, don't show the full contents of the schema. |
| N/A | options.source | `string` | None | The absolute location or URL of the source file to use as the base to resolve relative references ($refs) from; required if options.resolve is set to true. For CLI commands, Widdershins uses the input file as the base for the $refs. |
| --summary | options.tocSummary | `boolean` | `false` | Use the operation summary as the TOC entry instead of the ID. |
| --useBodyName | options.useBodyName | `boolean` | Use original param name for OpenAPI 2.0 body parameter. |
| -v, --verbose | options.verbose | `boolean` | `false` | Increase verbosity. |
| -h, --help | options.help | `boolean` | `false` | Show help. |
| --version | options.version | `boolean` | `false` | Show version number. |
| -c, --code | options.codeSamples | `boolean` | `false` | Omit generated code samples. |
| --httpsnippet | options.httpsnippet | `boolean` | `false` | Use httpsnippet to generate code samples. |
| -d, --discovery | options.discovery | `boolean` | `false` | Include schema.org WebAPI discovery data. |
| -e, --environment | N/A | `string` | None | File to load config options from. |
| -i, --includes | options.includes | `string` | None | List of files to put in the `include` header of the output Markdown. Processors such as Shins can then import the contents of these files. |
| -l, --lang | options.lang | `boolean` | `false` | Generate the list of languages for code samples based on the languages used in the source file's `x-code-samples` examples. |
| --language_tabs | options.language_tabs | `string` | (Differs for each input type) | List of language tabs for code samples using language[:label[:client]] format, such as `javascript:JavaScript:request`. |
| -m, --maxDepth | options.maxDepth | `integer` | 10 | Maximum depth to show for schema examples. |
| -o, --outfile | N/A | `string` | None | File to write the output markdown to. If left blank, Widdershins sends the output to stdout. |
| -r, --raw | options.raw | `boolean` | `false` | Output raw schemas instead of example values. |
| -s, --search | options.search | `boolean` | `true` | Set the value of the `search` parameter in the header so Markdown processors like Shins include search or not in their output. |
| -t, --theme | options.theme | `string` | darkula | Syntax-highlighter theme to use. |
| -u, --user_templates | options.user_templates | `string` | None | Directory to load override templates from. |
| -x, --experimental | options.experimental | `boolean` |  | For backwards compatibility only; ignored. |
| -y, --yaml | options.yaml | `boolean` | `false` | Display JSON schemas in YAML format. |
|  | options.templateCallback | `function` | None | A `function` that is called before and after each template (JavaScript code only). |
|  | options.toc_footers | `object` | A map of `url`s and `description`s to be added to the ToC footers array (JavaScript code only). |

In Node.JS code, create an options object and pass it to the Widdershins `convert` function, as in this example:

```javascript
const converter = require('widdershins');
let options = {}; // defaults shown
options.codeSamples = true;
options.httpsnippet = false;
//options.language_tabs = [];
//options.language_clients = [];
//options.loadedFrom = sourceUrl; // only needed if input document is relative
//options.user_templates = './user_templates';
options.templateCallback = function(templateName,stage,data) { return data };
options.theme = 'darkula';
options.search = true;
options.sample = true; // set false by --raw
options.discovery = false;
options.includes = [];
options.shallowSchemas = false;
options.tocSummary = false;
options.headings = 2;
options.yaml = false;
//options.resolve = false;
//options.source = sourceUrl; // if resolve is true, must be set to full path or URL of the input document
converter.convert(apiObj,options,function(err,str){
  // str contains the converted markdown
});
```

To only include a subset of the pre-defined language-tabs, or to rename their display-names, you can override the `options.language_tabs`:

```javascript
options.language_tabs = [{ 'go': 'Go' }, { 'http': 'HTTP' }, { 'javascript': 'JavaScript' }, { 'javascript--node': 'Node.JS' }, { 'python': 'Python' }, { 'ruby': 'Ruby' }];
```

The `--environment` option specifies a JSON or YAML-formatted `options` object, for example:

```json
{
  "language_tabs": [{ "go": "Go" }, { "http": "HTTP" }, { "javascript": "JavaScript" }, { "javascript--node": "Node.JS" }, { "python": "Python" }, { "ruby": "Ruby" }],
  "verbose": true,
  "tagGroups": [
    {
      "title": "Companies",
      "tags": ["companies"]
    },
    {
      "title": "Billing",
      "tags": ["invoice-create", "invoice-close", "invoice-delete"]
    }
  ]
}
```

You can also use the environment file to group OAS/Swagger tagged paths together to create a more elegant table of contents, and overall page structure.

If you need to support a version of Slate \<v1.5.0 (or a renderer which also doesn't support display-names for language-tabs, such as `node-slate`, `slate-node` or `whiteboard`), you can use the `--environment` option with the included `whiteboard_env.json` file to simply achieve this.

If you are using the `httpsnippet` option to generate code samples, you can specify the client library used to perform the requests for each language by overriding the `options.language_clients`:

```javascript
options.language_clients = [{ 'shell': 'curl' }, { 'node': 'request' }, { 'java': 'unirest' }];
```

If the language name differs between the markdown name required to syntax highlight and the httpsnippet required target, both can be specified in the form `markdown--target`.

To see the list of languages and clients supported by httpsnippet, [click here](https://github.com/Kong/httpsnippet/tree/master/src/targets).

The `loadedFrom` option is only needed where the OpenAPI / Swagger definition does not specify a host, and (as per the OpenAPI [specification](https://github.com/OAI/OpenAPI-Specification/blob/master/versions/2.0.md#fixed-fields)) the API endpoint is deemed to be based on the source URL
the definition was loaded from.

To see the list of highlight-js syntax highlighting themes, [click here](https://highlightjs.org/static/demo/).

Schema.org WebAPI discovery data is included if the `discovery` option above is set `true`. See the W3C [WebAPI Discovery Community Group](https://www.w3.org/community/web-api-discovery/) for more information.

## Language tabs

Widdershins supports the `x-code-samples` [vendor-extension](https://github.com/Rebilly/ReDoc/blob/master/docs/redoc-vendor-extensions.md#operation-object-vendor-extensions) to completely customise your documentation. Alternatively, you can edit the default code-samples in the `templates` sub-directory, or override them using the `user_templates` option to specify a directory containing your templates.

Widdershins supports the use of multiple language tabs with the same language (i.e. plain Javascript and Node.Js). To use this support you must be using Slate (or one of its ports compatible with) version 1.5.0 or higher. [Shins](https://github.com/mermade/shins) versions track Slate version numbers.

## Templates

By default, Widdershins uses the templates in its `templates/` folder to generate the Markdown output. To customize the templates, copy some or all of them to a folder and pass their location to the `user_templates` parameter.

The templates include `.dot` templates and `.def` partials. To override a `.dot` template, you must copy it and the child `.def` partials that the template references. Similarly, to override a `.def` partial, you must also copy the parent `.dot` template. For OpenAPI 3, the primary template is `main.dot` and its main child partials are `parameters.def`, `responses.def`, and `callbacks.def`.

This means that it is usually easiest to copy all `.dot` and `.def` files to your user templates directory so you don't skip a template or partial. To bring in changes from Widdershins updates, you can use a visual `diff` tool which can run across two directories, such as [Meld](http://meldmerge.org/) or [WinMerge](http://winmerge.org).

### Template syntax

Templates are compiled with [doT.js](https://github.com/olado/doT#readme).

Templates have access to a `data` object with a range of properties based on the document context. For information about the parameters, see the README file for the appropriate templates:

* [Swagger 2.0 / OpenAPI 3.0.x template parameters](/templates/openapi3/README.md)
* [AsyncAPI 1.x template parameters](/templates/asyncapi1/README.md)
* [Semoasa 0.1.0 template parameters](/templates/semoasa/README.md)

To print the value of a parameter or variable in a template, use the code `{{=parameterName}}`. For example, to print the title of an OpenAPI 3 spec (from its `info.title` field), use the code `{{=data.api.info.title}}`.

To loop through values in an array, use the code `{{~ arrayName :tempVariable}}` to start the loop and the code `{{~}}` to close the loop. For example, the OpenAPI 3 partial `parameters.def` uses this code to create a table of the parameters in an operation:
```
|Name|In|Type|Required|Description|
|---|---|---|---|---|
{{~ data.parameters :p}}|{{=p.name}}|{{=p.in}}|{{=p.safeType}}|{{=p.required}}|{{=p.shortDesc || 'none'}}|
{{~}}
```

For if/then logic, use the code `{{? booleanExpression}}` to start the code block and the code `{{?}}` to close the block. For example, the OpenAPI 3 `main.dot` template calls the `security.def` partial to show information about the security schemes if the OpenAPI spec includes a `securitySchemes` section:
```
{{? data.api.components && data.api.components.securitySchemes }}
{{#def.security}}
{{?}}
```

You can run arbitrary JavaScript within a template by inserting a code block within curly braces. For example, this code creates a variable and references it with normal doT.js syntax later in the template:
```
{{ {
let message = "Hello!";
} }}

{{=message}}
```

### Template callbacks

The `templateCallback` parameter points to a function that Widdershins calls before and after each template runs. The callback function receives a `data` object that contains the spec that Widdershins is processing; the function must return this object. You can use callback functions only if you are calling Widdershins from JavaScript code, not from the command line.

Widdershins passes these variables to the callback function:
- `templateName`: The name of the template, such as `main`.
- `stage`: Whether Widdershins is calling the callback function before (`pre`) or after (`post`) the template.
- `data`: An object that contains the data that Widdershins is processing. You can mutate the `data` object in any way you see fit, but the function must return it whether it changes it or not. Content that you put in the `data.append` property is appended to the current output stream.

For example, this JavaScript code prints the name of the template and the processing stage in the output Markdown:
```javascript
'use strict';

const converter = require('widdershins');
const fs = require('fs');

let options = {};
options.templateCallback = myCallBackFunction;

function myCallBackFunction(templateName, stage, data) {
  let statusString = "Template name: " + templateName + "\n";
  statusString += "Stage: " + stage + "\n";
  data.append = statusString;
  return data;
}

const apiObj = JSON.parse(fs.readFileSync('defs/petstore3.json'));

converter.convert(apiObj, options)
.then(str => {
  fs.writeFileSync('petstore3Output.md', str, 'utf8');
});
```

## Tests

To run a test-suite:

```
node testRunner {path-to-APIs}
```

The test harness currently expects `.yaml` or `.json` files and has been tested against

* [APIs.guru](https://github.com/APIs-guru/OpenAPI-directory)
* [Mermade OpenAPI definitions collection](https://github.com/mermade/OpenAPI-definitions)

### Comparison between this and other OpenAPI / Swagger to Slate tools

[Blog posting](https://dev.to/mikeralphson/comparison-of-various-openapiswagger-to-slate-conversion-tools) by the author of Widdershins.

### Acknowledgements

* [@latgeek](https://github.com/LatGeek) for the logo.
* [@vfernandestoptal](https://github.com/vfernandestoptal) for the httpsnippet support.

### Widdershins in the wild

Please feel free to add a link to your API documentation here.

* [GOV.UK Content API v1.0.0](https://content-api.publishing.service.gov.uk/reference.html)
* [GOV UK Digital Marketplace API v1.0.0](https://alphagov.github.io/digitalmarketplace-api-docs/#digital-marketplace-api-v1-0-0)
* [Capital One API](https://www.capitalone.co.uk/developer/api/)
* [Cognite Data API](http://doc.cognitedata.com/)
* [SpeckleWorks API](https://speckleworks.github.io/SpeckleSpecs)
* [Bank by API](https://tbicr.github.io/bank-api/bank-api.html)
* [Open EO API](https://open-eo.github.io/openeo-api-poc/apireference/index.html)
* [Split Payments API](http://docs.split.cash/)
* [LeApp daemon API](https://leapp-to.github.io/shins/index.html)
* [Shutterstock API](https://api-reference.shutterstock.com/)
* [Shotstack Video Editing API](https://shotstack.io/docs/api/index.html)

### Widdershins and Shins

* `Widdershins` works well with Slate, but for a solely Node.js-based experience, why not try the [Shins](https://github.com/mermade/shins) port?
