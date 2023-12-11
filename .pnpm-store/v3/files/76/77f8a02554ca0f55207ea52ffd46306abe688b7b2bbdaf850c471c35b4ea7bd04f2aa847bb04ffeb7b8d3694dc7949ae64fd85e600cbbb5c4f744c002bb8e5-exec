#!/usr/bin/env node

'use strict';
const fs = require('fs');
const path = require('path');
const url = require('url');

const yaml = require('yaml');
const fetch = require('node-fetch');

const converter = require('./lib/index.js');

var argv = require('yargs')
    .usage('widdershins [options] {input-file|url} [[-o] output markdown]')
    .demand(1)
    .strict()
    .string('abstract')
    .alias('a','abstract')
    .describe('abstract','The filename of the Markdown include file to use for the ReSpec abstract section.')
    .default('abstract','./include/abstract.md')
    .boolean('code')
    .alias('c','code')
    .describe('code','Omit generated code samples.')
    .string('customApiKeyValue')
    .describe('customApiKeyValue','Set a custom API key value to use as the API key in generated code examples.')
    .string('includes')
    .boolean('discovery')
    .alias('d','discovery')
    .describe('discovery','Include schema.org WebAPI discovery data.')
    .string('environment')
    .alias('e','environment')
    .describe('environment','File to load config options from.')
    .boolean('expandBody')
    .describe('expandBody','Expand the schema and show all properties in the request body.')
    .number('headings')
    .describe('headings','Set the value of the `headingLevel` parameter in the header so Shins knows how many heading levels to show in the table of contents.')
    .default('headings',2)
    .boolean('httpsnippet')
    .default('httpsnippet',false)
    .describe('httpsnippet','Use httpsnippet to generate code samples.')
    .boolean('html')
    .describe('html','Output html instead of Markdown; implies omitHeader.')
    .alias('i','includes')
    .describe('includes','List of files to put in the `include` header of the output Markdown.')
    .boolean('lang')
    .alias('l','lang')
    .describe('lang','Generate the list of languages for code samples based on the languages used in the source file\'s `x-code-samples` examples.')
    .array('language_tabs')
    .describe('language_tabs', 'List of language tabs for code samples using "language[:label[:client]]" format, such as `javascript:JavaScript:request`.')
    .number('maxLevel')
    .alias('m','maxDepth')
    .describe('maxDepth','Maximum depth to show for schema examples.')
    .default('maxDepth',10)
    .boolean('omitBody')
    .describe('omitBody','Omit the body parameter from the parameters table.')
    .boolean('omitHeader')
    .describe('omitHeader','Omit the header / YAML front-matter in the generated Markdown file.')
    .string('outfile')
    .alias('o','outfile')
    .describe('outfile','File to write the output markdown to. If left blank, Widdershins sends the output to stdout.')
    .boolean('raw')
    .alias('r','raw')
    .describe('raw','Output raw schemas instead of example values.')
    .boolean('resolve')
    .describe('resolve','Resolve external $refs')
    .string('respec')
    .describe('respec','Filename containing the ReSpec config object; implies html and omitHeader.')
    .boolean('search')
    .alias('s','search')
    .default('search',true)
    .describe('search','Set the value of the `search` parameter in the header so Markdown processors like Shins include search or not in their output.')
    .boolean('shallowSchemas')
    .describe('shallowSchemas',"When referring to a schema with a $ref, don't show the full contents of the schema.")
    .string('sotd')
    .describe('sotd','The filename of the markdown include file to use for the ReSpec SotD section.')
    .default('sotd','./include/sotd.md')
    .boolean('summary')
    .describe('summary','Use the operation summary as the TOC entry instead of the ID.')
    .string('theme')
    .alias('t','theme')
    .describe('theme','Syntax-highlighter theme to use.')
    .boolean('useBodyName')
    .describe('useBodyName','Use original param name for OpenAPI 2.0 body parameter')
    .string('user_templates')
    .alias('u','user_templates')
    .describe('user_templates','Directory to load override templates from.')
    .boolean('verbose')
    .alias('v','verbose')
    .describe('verbose','Increase verbosity.')
    .boolean('experimental')
    .alias('x','experimental')
    .describe('experimental','For backwards compatibility only; ignored.')
    .boolean('yaml')
    .alias('y','yaml')
    .describe('yaml','Display JSON schemas in YAML format.')
    .help('h')
    .alias('h','Show help.')
    .version()
    .argv;

var options = {};

async function doit(s) {
    var api = {};
    try {
        api = yaml.parse(s);
    }
    catch(ex) {
        console.error('Failed to parse YAML/JSON, falling back to API Blueprint');
        console.error(ex.message);
        api = s;
    }

    try {
        let output = await converter.convert(api,options);
        let outfile = argv.outfile||argv._[1];
        if (outfile) {
            fs.writeFileSync(path.resolve(outfile),output,'utf8');
        }
        else {
            console.log(output);
        }
    }
    catch (err) {
        console.warn(err);
    }
}

options.codeSamples = !argv.code;
options.httpsnippet = argv.httpsnippet;
if (argv.lang) {
    options.language_tabs = [];
}
else if (argv.language_tabs) {
    if (!options.language_clients) options.language_clients = [];
    const languages = argv.language_tabs
        .reduce((languages, item) => {
            const [lang, name, client] = item.split(':', 3);

            languages.language_tabs.push({ [lang]: name || lang });
            languages.language_clients.push({ [lang]: client || '' });

            return languages;
        }, { language_tabs: [], language_clients: []});
    options.language_tabs = languages.language_tabs;
    options.language_clients = languages.language_clients;
}
if (argv.theme) options.theme = argv.theme;
options.user_templates = argv.user_templates;
options.inline = argv.inline;
options.sample = !argv.raw;
options.discovery = argv.discovery;
options.verbose = argv.verbose;
if (options.verbose > 2) Error.stackTraceLimit = Infinity;
options.tocSummary = argv.summary;
options.headings = argv.headings;
options.experimental = argv.experimental;
options.resolve = argv.resolve;
options.expandBody = argv.expandBody;
options.maxDepth = argv.maxDepth;
options.omitBody = argv.omitBody;
options.omitHeader = argv.omitHeader;
options.shallowSchemas = argv.shallowSchemas;
options.yaml = argv.yaml;
options.customApiKeyValue = argv.customApiKeyValue;
options.html = argv.html;
options.respec = argv.respec;
options.useBodyName = argv.useBodyName;
if (argv.search === false) options.search = false;
if (argv.includes) options.includes = argv.includes.split(',');
if (argv.respec) {
    options.abstract = argv.abstract;
    options.sotd = argv.sotd;
    let r = fs.readFileSync(path.resolve(argv.respec),'utf8');
    try {
        options.respec = yaml.parse(r);
    }
    catch (ex) {
        console.error(ex.message);
    }
}
if (options.respec) options.html = true;
if (options.html) options.omitHeader = true;

if (argv.environment) {
    var e = fs.readFileSync(path.resolve(argv.environment),'utf8');
    var env = {};
    try {
        env = yaml.parse(e);
    }
    catch (ex) {
        console.error(ex.message);
    }
    options = Object.assign({},options,env);
}

var input = argv._[0];
options.source = input;
var up = url.parse(input);
if (up.protocol && up.protocol.startsWith('http')) {
    fetch(input)
    .then(function (res) {
        return res.text();
    }).then(function (body) {
        doit(body);
    }).catch(function (err) {
        console.error(err.message);
    });
}
else {
    let s = fs.readFileSync(input,'utf8');
    doit(s);
}

