'use strict';

const path = require('path');

const yaml = require('yaml');
const dot = require('dot');
dot.templateSettings.strip = false;
dot.templateSettings.varname = 'data';

const common = require('./common.js');
const dereference = require('reftools/lib/dereference.js').dereference;
const oas_descs = require('../resources/oas_descs.js');

let templates = {};

function preProcessor(api) {
    return api;
}

function convert(api, options) {
    return new Promise(function (resolve, reject) {
        let templates;
        api = preProcessor(api);

        let defaults = {};
        defaults.includes = [];
        defaults.search = true;
        defaults.theme = 'darkula';

        options = Object.assign({}, defaults, options);

        let header = {};
        header.title = (api.info ? api.info.title : 'Semoasa documentation');

        header.language_tabs = ['json', 'yaml'];

        header.toc_footers = [];
        if (api.externalDocs) {
            if (api.externalDocs.url) {
                header.toc_footers.push('<a href="' + api.externalDocs.url + '">' + (api.externalDocs.description ? api.externalDocs.description : 'External Docs') + '</a>');
            }
        }
        if (options.toc_footers) {
            for (var key in options.toc_footers) {
                header.toc_footers.push('<a href="' + options.toc_footers[key].url + '">' + options.toc_footers[key].description + '</a>');
            }
        }
        header.includes = options.includes;
        header.search = options.search;
        header.highlight_theme = options.theme;

        if (typeof templates === 'undefined') {
            templates = dot.process({ path: path.join(__dirname, '..', 'templates', 'semoasa') });
        }
        if (options.user_templates) {
            templates = Object.assign(templates, dot.process({ path: options.user_templates }));
        }

        let data = {};
        if (options.verbose) console.log('starting deref', api.info.title);
        data.api = dereference(api, api, { verbose: options.verbose, $ref: 'x-widdershins-oldRef' });
        if (options.verbose) console.log('finished deref');
        data.options = options;
        data.header = header;
        data.templates = templates;
        data.translations = {};
        templates.translations(data);
        data.oas2_descs = oas_descs.oas2_descs;
        data.oas3_descs = oas_descs.oas3_descs;
        data.utils = {};
        data.utils.yaml = yaml;
        data.utils.getSample = common.getSample;
        data.utils.schemaToArray = common.schemaToArray;
        data.utils.slugify = common.slugify;
        data.utils.linkCase = function (s) {
            return s[0].toLowerCase() + s.substr(1);
        };
        data.utils.join = function (s) {
            return s.split('\r').join('').split('\n').join('<br>').trim();
        };

        let content;
        try {
            content = options.omitHeader ? '' : '---\n' + yaml.stringify(header) + '\n---\n\n';
            content += templates.main(data);
        }
        catch (ex) {
            return reject(ex);
        }
        content = common.removeDupeBlankLines(content);

        if (options.html) content = common.html(content, header, options);

        resolve(content);
    });
}

module.exports = {
    convert: convert
};
