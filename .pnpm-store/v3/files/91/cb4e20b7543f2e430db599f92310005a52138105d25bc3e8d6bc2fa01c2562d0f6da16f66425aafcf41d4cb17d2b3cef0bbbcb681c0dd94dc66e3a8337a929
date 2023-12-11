'use strict';

const path = require('path');
const util = require('util');
const up = require('url');

const yaml = require('yaml');
const safejson = require('fast-safe-stringify');
const uri = require('urijs');
const URITemplate = require('urijs/src/URITemplate');
const dot = require('dot');
dot.templateSettings.strip = false;
dot.templateSettings.varname = 'data';

const xml = require('jgexml/json2xml.js');
const jptr = require('reftools/lib/jptr.js').jptr;
const dereference = require('reftools/lib/dereference.js').dereference;
const clone = require('reftools/lib/clone.js').circularClone;
const swagger2openapi = require('swagger2openapi');

const common = require('./common.js');

let templates;

function convertToToc(source, data) {
    let resources = {};
    resources[data.translations.defaultTag] = { count: 0, methods: {} };
    if (source.tags) {
        for (let tag of source.tags) {
            resources[tag.name] = { count: 0, methods: {}, description: tag.description, externalDocs: tag.externalDocs };
        }
    }
    for (var p in source.paths) {
        if (!p.startsWith('x-')) {
            for (var m in source.paths[p]) {
                if ((m !== 'parameters') && (m !== 'summary') && (m !== 'description') && (!m.startsWith('x-'))) {
                    var method = {};
                    method.operation = source.paths[p][m];
                    method.pathItem = source.paths[p];
                    method.verb = m;
                    method.path = p;
                    method.pathParameters = source.paths[p].parameters;
                    var sMethodUniqueName = (method.operation.operationId ? method.operation.operationId : m + '_' + p).split('?')[0].split('/').join('_');
                    if (data.options.tocSummary && method.operation.summary) {
                        sMethodUniqueName = method.operation.summary;
                    }
                    method.slug = sMethodUniqueName.toLowerCase().split(' ').join('-'); // TODO {, } and : ?
                    var tagName = data.translations.defaultTag;
                    var tagDescription = '';
                    if (method.operation.tags && method.operation.tags.length > 0) {
                        var tagData = getTagGroup(method.operation.tags[0], data.options.tagGroups);
                        tagName = tagData.name;
                        tagDescription = tagData.description;
                    }
                    if (!resources[tagName]) {
                        resources[tagName] = { count: 0, methods: {}, description: tagDescription };
                    }
                    resources[tagName].count++;
                    resources[tagName].methods[sMethodUniqueName] = method;
                }
            }
        }
    }
    for (let r in resources) {
        if (resources[r].count <= 0) delete resources[r];
    }
    return resources;
}

function getTagGroup(tag, tagGroups) {
    if (tagGroups) {
        for (let group of tagGroups) {
            if (group.tags.indexOf(tag) > -1) {
                return { name: group.title, description: group.description };
            }
        }
    }
    return { name: tag, description: '' };
}

function fakeProdCons(data) {
    data.produces = [];
    data.consumes = [];
    data.bodyParameter = {};
    data.bodyParameter.exampleValues = {};
    for (var r in data.operation.responses) {
        var response = data.operation.responses[r];
        if (!r.startsWith('x-')) {
            for (var prod in response.content) {
                if (!data.produces.includes(prod)) {
                    data.produces.push(prod);
                }
            }
        }
    }
    let op = data.method.operation;
    if (op.requestBody) {
        for (var rb in op.requestBody.content) {
            data.consumes.push(rb);
            if (!data.bodyParameter.exampleValues.object) {
                data.bodyParameter.present = true;
                data.bodyParameter.contentType = rb;
                if (op.requestBody.content[rb].schema) {
                    let schema = op.requestBody.content[rb].schema;
                    if (schema['x-widdershins-oldRef']) {
                        data.bodyParameter.refName = schema['x-widdershins-oldRef'].replace('#/components/schemas/', '');
                    }
                    else {
                        if ((schema.type === 'array') && (schema.items) && (schema.items['x-widdershins-oldRef'])) {
                            data.bodyParameter.refName = schema.items['x-widdershins-oldRef'].replace('#/components/schemas/', '');
                        }

                    }
                }
                data.bodyParameter.schema = op.requestBody.content[rb].schema;
                if (op.requestBody.content[rb].examples) {
                    let key = Object.keys(op.requestBody.content[rb].examples)[0];
                    data.bodyParameter.exampleValues.object = op.requestBody.content[rb].examples[key].value;
                    data.bodyParameter.exampleValues.description = op.requestBody.content[rb].examples[key].description;
                }
                else {
                    data.bodyParameter.exampleValues.object = common.getSample(op.requestBody.content[rb].schema, data.options, { skipReadOnly: true, quiet: true }, data.api);
                }
                if (typeof data.bodyParameter.exampleValues.object === 'object') {
                    data.bodyParameter.exampleValues.json = safejson(data.bodyParameter.exampleValues.object, null, 2);
                }
                else {
                    data.bodyParameter.exampleValues.json = data.bodyParameter.exampleValues.object;
                }
            }
        }
    }
}

function getParameters(data) {

    function stupidity(varname) {
        let s = encodeURIComponent(varname);
        s = s.split('-').join('%2D');
        s = s.split('$').join('%24');
        s = s.split('.').join('%2E');
        s = s.split('(').join('%28');
        s = s.split(')').join('%29');
        return s;
    }

    data.allHeaders = [];
    data.headerParameters = [];
    data.requiredParameters = [];
    let uriTemplateStr = data.method.path.split('?')[0].split('/ /').join('/+/');
    let requiredUriTemplateStr = uriTemplateStr;
    var templateVars = {};

    if (data.consumes.length) {
        var contentType = {};
        contentType.name = 'Content-Type';
        contentType.type = 'string';
        contentType.in = 'header';
        contentType.exampleValues = {};
        contentType.exampleValues.json = "'" + data.consumes[0] + "'";
        contentType.exampleValues.object = data.consumes[0];
        data.allHeaders.push(contentType);
    }
    if (data.produces.length) {
        var accept = {};
        accept.name = 'Accept';
        accept.type = 'string';
        accept.in = 'header';
        accept.exampleValues = {};
        accept.exampleValues.json = "'" + data.produces[0] + "'";
        accept.exampleValues.object = data.produces[0];
        data.allHeaders.push(accept);
    }

    if (!Array.isArray(data.parameters)) data.parameters = [];
    data.longDescs = false;
    for (let param of data.parameters) {
        param.exampleValues = {};
        if (!param.required) param.required = false;
        let pSchema = param.schema;
        if (!pSchema && param.content) {
            pSchema = Object.values(param.content)[0].schema;
        }
        if (pSchema && !param.safeType) {
            param.originalType = pSchema.type;
            param.safeType = pSchema.type || common.inferType(pSchema);
            if (pSchema.format) {
                param.safeType = param.safeType + '(' + pSchema.format + ')';
            }
            if ((param.safeType === 'array') && (pSchema.items)) {
                let itemsType = pSchema.items.type;
                if (!itemsType) {
                    itemsType = common.inferType(pSchema.items);
                }
                param.safeType = 'array[' + itemsType + ']';
            }
            if (pSchema["x-widdershins-oldRef"]) {
                let schemaName = pSchema["x-widdershins-oldRef"].replace('#/components/schemas/', '');
                param.safeType = '[' + schemaName + '](#schema' + schemaName.toLowerCase() + ')';
            }
            if (param.refName) param.safeType = '[' + param.refName + '](#schema' + param.refName.toLowerCase() + ')';
        }
        if (pSchema) {
            param.exampleValues.object = param.example || param.default || common.getSample(pSchema, data.options, { skipReadOnly: true, quiet: true }, data.api);
            if (typeof param.exampleValues.object === 'object') {
                param.exampleValues.json = safejson(param.exampleValues.object, null, 2);
            }
            else {
                param.exampleValues.json = "'" + param.exampleValues.object + "'";
            }
        }
        if (param.description === 'undefined') { // yes, the string
            param.description = '';
        }
        if ((typeof param.description !== 'undefined') && (typeof param.description === 'string')) {
            param.shortDesc = param.description.split('\n')[0];
            if (param.shortDesc !== param.description) data.longDescs = true;
        }

        if (param.in === 'cookie') {
            if (!param.style) param.style = 'form';
            // style prefixes: form
        }
        if (param.in === 'header') {
            if (!param.style) param.style = 'simple';
            data.headerParameters.push(param);
            data.allHeaders.push(param);
        }
        if (param.in === 'path') {
            let template = param.allowReserved ? '{+' : '{';
            // style prefixes: matrix, label, simple
            if (!param.style) param.style = 'simple';
            if (param.style === 'label') template += '.';
            if (param.style === 'matrix') template += ';';
            template += stupidity(param.name);
            template += param.explode ? '*}' : '}';
            uriTemplateStr = uriTemplateStr.split('{' + param.name + '}').join(template);
            requiredUriTemplateStr = requiredUriTemplateStr.split('{' + param.name + '}').join(template);
        }
        if (param.in === 'query') {
            let isFirst = uriTemplateStr.indexOf('{&') < 0;
            // Since RFC6570 doesn't support multiple operators we cannot use (?+ and (&+ for reserved parameters
            let prefix = isFirst ? '{?' : '{&';
            var template = '';
            // style prefixes: form, spaceDelimited, pipeDelimited, deepObject
            if (!param.style) param.style = 'form';
            template += stupidity(param.name);
            template += param.explode ? '*}' : '}';
            uriTemplateStr += (prefix + template);

            if (param.required) {
                let isFirstRequired = requiredUriTemplateStr.indexOf('{?') < 0;
                let reqPrefix = isFirstRequired ? '{?' : '{&';
                requiredUriTemplateStr += (reqPrefix + template);
                data.requiredParameters.push(param);
            }
        }
        templateVars[stupidity(param.name)] = param.exampleValues.object;
    }

    let effSecurity;
    let existingAuth = data.allHeaders.find(function (e, i, a) {
        return e.name.toLowerCase() === 'authorization';
    });
    if (data.operation.security) {
        if (data.operation.security.length) {
            effSecurity = Object.keys(data.operation.security[0]);
        }
    }
    else if (data.api.security && data.api.security.length) {
        effSecurity = Object.keys(data.api.security[0]);
    }
    if (effSecurity && effSecurity.length && data.api.components && data.api.components.securitySchemes) {
        for (let ess of effSecurity) {
            if (data.api.components.securitySchemes[ess]) {
                let secScheme = data.api.components.securitySchemes[ess];
                if (!existingAuth && ((secScheme.type === 'oauth2') || (secScheme.type === 'openIdConnect') ||
                    ((secScheme.type === 'http') && (secScheme.scheme === 'bearer')))) {
                    let authHeader = {};
                    authHeader.name = 'Authorization';
                    authHeader.type = 'string';
                    authHeader.in = 'header';
                    authHeader.isAuth = true;
                    authHeader.exampleValues = {};
                    authHeader.exampleValues.object = 'Bearer {access-token}';
                    authHeader.exampleValues.json = "'" + authHeader.exampleValues.object + "'";
                    data.allHeaders.push(authHeader);
                }
                else if ((secScheme.type === 'apiKey') && (secScheme.in === 'header')) {
                    let authHeader = {};
                    authHeader.name = secScheme.name;
                    authHeader.type = 'string';
                    authHeader.in = 'header';
                    authHeader.isAuth = true;
                    authHeader.exampleValues = {};
                    authHeader.exampleValues.object = 'API_KEY';
                    if (data.options.customApiKeyValue) {
                        authHeader.exampleValues.object = data.options.customApiKeyValue;
                    }
                    authHeader.exampleValues.json = "'" + authHeader.exampleValues.object + "'";
                    data.allHeaders.push(authHeader);
                }
            }
        }
    }

    let uriTemplate = new URITemplate(uriTemplateStr);
    let requiredUriTemplate = new URITemplate(requiredUriTemplateStr);
    data.uriExample = uriTemplate.expand(templateVars);
    data.requiredUriExample = requiredUriTemplate.expand(templateVars);

    //TODO deconstruct and reconstruct to cope w/ spaceDelimited/pipeDelimited

    data.queryString = data.uriExample.substr(data.uriExample.indexOf('?'));
    if (!data.queryString.startsWith('?')) data.queryString = '';
    data.queryString = data.queryString.split('%25').join('%');
    data.requiredQueryString = data.requiredUriExample.substr(data.requiredUriExample.indexOf('?'));
    if (!data.requiredQueryString.startsWith('?')) data.requiredQueryString = '';
    data.requiredQueryString = data.requiredQueryString.split('%25').join('%');
}

function getBodyParameterExamples(data) {
    let obj = data.bodyParameter.exampleValues.object;
    let content = '';
    let xmlWrap = false;
    if (data.bodyParameter.schema && data.bodyParameter.schema.xml) {
        xmlWrap = data.bodyParameter.schema.xml.name;
    }
    else if (data.bodyParameter.schema && data.bodyParameter.schema["x-widdershins-oldRef"]) {
        xmlWrap = data.bodyParameter.schema["x-widdershins-oldRef"].split('/').pop();
    }
    if (common.doContentType(data.consumes, 'json')) {
        content += '```json\n';
        content += safejson(obj, null, 2) + '\n';
        content += '```\n\n';
    }
    if (common.doContentType(data.consumes, 'yaml')) {
        content += '```yaml\n';
        content += yaml.stringify(obj) + '\n';
        content += '```\n\n';
    }
    if (common.doContentType(data.consumes, 'text')) {
        content += '```\n';
        content += yaml.stringify(obj) + '\n';
        content += '```\n\n';
    }
    if (common.doContentType(data.consumes, 'form')) {
        content += '```yaml\n';
        content += yaml.stringify(obj) + '\n';
        content += '```\n\n';
    }
    if (common.doContentType(data.consumes, 'xml') && (typeof obj === 'object')) {
        if (xmlWrap) {
            var newObj = {};
            newObj[xmlWrap] = obj;
            obj = newObj;
        }
        content += '```xml\n';
        content += xml.getXml(JSON.parse(safejson(obj)), '@', '', true, '  ', false) + '\n';
        content += '```\n\n';
    }
    return content;
}

function fakeBodyParameter(data) {
    if (!data.parameters) data.parameters = [];
    let bodyParams = [];
    if (data.bodyParameter.schema) {
        let param = {};
        param.in = 'body';
        param.schema = data.bodyParameter.schema;
        param.name = 'body';
        if (data.operation.requestBody) {
            param.required = data.operation.requestBody.required || false;
            param.description = data.operation.requestBody.description;
            if (data.options.useBodyName && data.operation['x-body-name']) {
                param.name = data.operation['x-body-name'];
            }
        }
        param.refName = data.bodyParameter.refName;
        if (!data.options.omitBody || param.schema["x-widdershins-oldRef"]) {
            bodyParams.push(param);
        }

        if ((param.schema.type === 'object') && (data.options.expandBody || (!param.schema["x-widdershins-oldRef"]))) {
            let offset = (data.options.omitBody ? -1 : 0);
            let props = common.schemaToArray(data.bodyParameter.schema, offset, { trim: true }, data);

            for (let block of props) {
                for (let prop of block.rows) {
                    let param = {};
                    param.in = 'body';
                    param.schema = prop.schema;
                    param.name = prop.displayName;
                    param.required = prop.required;
                    param.description = prop.description;
                    param.safeType = prop.safeType;
                    param.depth = prop.depth;
                    bodyParams.push(param);
                }
            }
        }

        if (!data.parameters || !Array.isArray(data.parameters)) data.parameters = [];
        data.parameters = data.parameters.concat(bodyParams);
    }
}

function mergePathParameters(data) {
    if (!data.parameters || !Array.isArray(data.parameters)) data.parameters = [];
    data.parameters = data.parameters.concat(data.method.pathParameters || []);
    data.parameters = data.parameters.filter((param, index, self) => self.findIndex((p) => { return p.name === param.name && p.in === param.in; }) === index || param.in === 'body');
}

function getResponses(data) {
    let responses = [];
    for (let r in data.operation.responses) {
        if (!r.startsWith('x-')) {
            let response = data.operation.responses[r];
            let entry = {};
            entry.status = r;
            entry.meaning = (r === 'default' ? data.translations.responseDefault : data.translations.responseUnknown);
            var url = '';
            for (var s in common.statusCodes) {
                if (common.statusCodes[s].code === r) {
                    entry.meaning = common.statusCodes[s].phrase;
                    url = common.statusCodes[s].spec_href;
                    break;
                }
            }
            if (url) entry.meaning = '[' + entry.meaning + '](' + url + ')';
            entry.description = (typeof response.description === 'string' ? response.description.trim() : undefined);
            entry.schema = data.translations.schemaNone;
            for (let ct in response.content) {
                let contentType = response.content[ct];
                if (contentType.schema) {
                    entry.type = contentType.schema.type;
                    entry.schema = data.translations.schemaInline;
                }
                if (contentType.schema && contentType.schema["x-widdershins-oldRef"] && contentType.schema["x-widdershins-oldRef"].startsWith('#/components/')) {
                    let schemaName = contentType.schema["x-widdershins-oldRef"].replace('#/components/schemas/', '');
                    entry.schema = '[' + schemaName + '](#schema' + schemaName.toLowerCase() + ')';
                    entry.$ref = true;
                }
                else {
                    if (contentType.schema && contentType.schema.type && (contentType.schema.type !== 'object') && (contentType.schema.type !== 'array')) {
                        entry.schema = contentType.schema.type;
                    }
                }
            }
            entry.content = response.content;
            entry.links = response.links;
            responses.push(entry);
        }
    }
    return responses;
}

function convertExample(ex) {
    if (typeof ex === 'string') {
        try {
            return yaml.parse(ex);
        }
        catch (e) {
            return ex;
        }
    }
    else return ex;
}

function getResponseExamples(data) {
    let content = '';
    let examples = [];
    let autoDone = {};
    for (let resp in data.operation.responses) {
        if (!resp.startsWith('x-')) {
            let response = data.operation.responses[resp];
            for (let ct in response.content) {
                let contentType = response.content[ct];
                let cta = [ct];
                // support embedded examples
                if (contentType.examples) {
                    for (let ctei in contentType.examples) {
                        let example = contentType.examples[ctei];
                        examples.push({ description: example.description || response.description, value: common.clean(convertExample(example.value)), cta: cta });
                    }
                }
                else if (contentType.example) {
                    examples.push({ description: resp + ' ' + data.translations.response, value: common.clean(convertExample(contentType.example)), cta: cta });
                }
                else if (contentType.schema) {
                    let obj = contentType.schema;
                    let autoCT = '';
                    if (common.doContentType(cta, 'json')) autoCT = 'json';
                    if (common.doContentType(cta, 'yaml')) autoCT = 'yaml';
                    if (common.doContentType(cta, 'xml')) autoCT = 'xml';
                    if (common.doContentType(cta, 'text')) autoCT = 'text';

                    if (!autoDone[autoCT]) {
                        autoDone[autoCT] = true;
                        let xmlWrap = false;
                        if (obj && obj.xml && obj.xml.name) {
                            xmlWrap = obj.xml.name;
                        }
                        else if (obj["x-widdershins-oldRef"]) {
                            xmlWrap = obj["x-widdershins-oldRef"].split('/').pop();
                        }
                        examples.push({ description: resp + ' ' + data.translations.response, value: common.getSample(obj, data.options, { skipWriteOnly: true, quiet: true }, data.api), cta: cta, xmlWrap: xmlWrap });
                    }
                }
            }
        }
    }
    let lastDesc = '';
    for (let example of examples) {
        if (example.description && example.description !== lastDesc) {
            content += '> ' + example.description + '\n\n';
            lastDesc = example.description;
        }
        if (common.doContentType(example.cta, 'json')) {
            content += '```json\n';
            content += safejson(example.value, null, 2) + '\n';
            content += '```\n\n';
        }
        if (common.doContentType(example.cta, 'yaml')) {
            content += '```yaml\n';
            content += yaml.stringify(example.value) + '\n';
            content += '```\n\n';
        }
        if (common.doContentType(example.cta, 'text')) {
            content += '```\n';
            content += JSON.stringify(example.value) + '\n';
            content += '```\n\n';
        }
        let xmlObj = example.value;
        if (example.xmlWrap) {
            xmlObj = {};
            xmlObj[example.xmlWrap] = example.value;
        }
        if ((typeof xmlObj === 'object') && common.doContentType(example.cta, 'xml')) {
            content += '```xml\n';
            content += xml.getXml(JSON.parse(safejson(xmlObj)), '@', '', true, '  ', false) + '\n';
            content += '```\n\n';
        }
    }
    return content;
}

function getResponseHeaders(data) {
    let headers = [];
    for (let r in data.operation.responses) {
        if (!r.startsWith('x-')) {
            let response = data.operation.responses[r];
            if (response.headers) {
                for (let h in response.headers) {
                    let header = response.headers[h];
                    let entry = {};
                    entry.status = r;
                    entry.header = h;
                    entry.description = header.description;
                    entry.in = 'header';
                    entry.required = header.required;
                    entry.schema = header.schema || {};
                    entry.type = entry.schema.type;
                    entry.format = entry.schema.format;
                    headers.push(entry);
                }
            }
        }
    }
    return headers;
}

function getAuthenticationStr(data) {
    let list = '';
    for (let s in data.security) {
        let count = 0;
        for (let sse in Object.keys(data.security[s])) {
            let secName = Object.keys(data.security[s])[sse];
            let link = '#/components/securitySchemes/' + secName;
            let secDef = jptr(data.api, link);
            let sep = (count > 0) ? ' & ' : ', ';
            list += (list ? sep : '') + (secDef ? secName : data.translations.secDefNone);
            let scopes = data.security[s][secName];
            if (Array.isArray(scopes) && (scopes.length > 0)) {
                list += ' ( ' + data.translations.secDefScopes + ': ';
                for (let scope in scopes) {
                    list += scopes[scope] + ' ';
                }
                list += ')';
            }
            count++;
        }
        if (count === 0) { // 'null' security
            list += (list ? ', ' : '') + data.translations.secDefNone;
        }
    }
    return list;
}

function convertInner(api, options) {
    return new Promise(function (resolve, reject) {
        let defaults = {};
        defaults.title = 'API';
        defaults.language_tabs = [{ 'shell': 'Shell' }, { 'http': 'HTTP' }, { 'javascript': 'JavaScript' }, { 'ruby': 'Ruby' }, { 'python': 'Python' }, { 'php': 'PHP' }, { 'java': 'Java' }, { 'go': 'Go' }];
        defaults.toc_footers = [];
        defaults.includes = [];
        defaults.search = true;
        defaults.theme = 'darkula';
        defaults.headings = 2;
        defaults.templateCallback = function (template, stage, data) { return data; };

        options = Object.assign({}, defaults, options);

        let data = {};
        if (options.verbose) console.warn('starting deref', api.info.title);
        if (api.components) {
            data.components = clone(api.components);
        }
        else {
            data.components = {};
        }
        data.api = dereference(api, api, { verbose: options.verbose, $ref: 'x-widdershins-oldRef' });
        if (options.verbose) console.warn('finished deref');

        if (data.api.components && data.api.components.schemas && data.api.components.schemas["x-widdershins-oldRef"]) {
            delete data.api.components.schemas["x-widdershins-oldRef"];
        }

        if (typeof templates === 'undefined') {
            templates = dot.process({ path: path.join(__dirname, '..', 'templates', 'openapi3') });
        }
        if (options.user_templates) {
            templates = Object.assign(templates, dot.process({ path: options.user_templates }));
        }
        data.options = options;
        data.translations = {};
        templates.translations(data);

        data.version = (data.api.info && data.api.info.version && typeof data.api.info.version === 'string' && data.api.info.version.toLowerCase().startsWith('v') ? data.api.info.version : 'v' + (data.api.info ? data.api.info.version : 'v1.0.0'));

        let header = {};
        header.title = api.info && (api.info.title || 'API') + ' ' + data.version;
        header.language_tabs = options.language_tabs;
        if (options.language_clients) header.language_clients = options.language_clients;
        header.toc_footers = [];
        if (api.externalDocs) {
            if (api.externalDocs.url) {
                header.toc_footers.push('<a href="' + api.externalDocs.url + '">' + (api.externalDocs.description ? api.externalDocs.description : data.translations.externalDocs) + '</a>');
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
        header.headingLevel = options.headings;

        data.header = header;
        data.title_prefix = (data.api.info && data.api.info.version ? common.slugify((data.api.info.title || '').trim() || 'API') : '');
        data.templates = templates;
        data.resources = convertToToc(api, data);

        if (data.api.servers && data.api.servers.length) {
            data.servers = data.api.servers;
        }
        else if (options.loadedFrom) {
            data.servers = [{ url: options.loadedFrom }];
        }
        else {
            data.servers = [{ url: '//' }];
        }
        data.host = up.parse(data.servers[0].url).host;
        data.protocol = up.parse(data.servers[0].url).protocol;
        if (data.protocol) data.protocol = data.protocol.replace(':', '');
        data.baseUrl = data.servers[0].url;

        data.operationStack = [];

        data.utils = {};
        data.utils.yaml = yaml;
        data.utils.inspect = util.inspect;
        data.utils.safejson = safejson;
        data.utils.isPrimitive = function (t) { return (t && (t !== 'object') && (t !== 'array')) };
        data.utils.toPrimitive = common.toPrimitive;
        data.utils.slashes = function (s) { return s.replace(/\/+/g, '/').replace(':/', '://'); };
        data.utils.slugify = common.slugify;
        data.utils.getSample = common.getSample;
        data.utils.schemaToArray = common.schemaToArray;
        data.utils.fakeProdCons = fakeProdCons;
        data.utils.getParameters = getParameters;
        data.utils.getCodeSamples = common.getCodeSamples;
        data.utils.getBodyParameterExamples = getBodyParameterExamples;
        data.utils.fakeBodyParameter = fakeBodyParameter;
        data.utils.mergePathParameters = mergePathParameters;
        data.utils.getResponses = getResponses;
        data.utils.getResponseExamples = getResponseExamples;
        data.utils.getResponseHeaders = getResponseHeaders;
        data.utils.getAuthenticationStr = getAuthenticationStr;
        data.widdershins = require('../package.json');
        data.utils.join = function (s) {
            return s.split('\r').join('').split('\n').join(' ').trim();
        };

        let content = '';
        if (!options.omitHeader) content += '---\n' + yaml.stringify(header) + '\n---\n\n';
        data = options.templateCallback('main', 'pre', data);
        if (data.append) { content += data.append; delete data.append; }
        try {
            content += templates.main(data);
        }
        catch (ex) {
            throw ex;
        }
        data = options.templateCallback('main', 'post', data);
        if (data.append) { content += data.append; delete data.append; }
        content = common.removeDupeBlankLines(content);

        if (options.html) content = common.html(content, header, options);

        resolve(content);
    });
}

function convert(api, options) {
    if (options.resolve) {
        return swagger2openapi.convertObj(api, { resolve: true, source: options.source, verbose: options.verbose })
        .then(sOptions => {
            return convertInner(sOptions.openapi, options);
        })
        .catch(err => {
            console.error(err.message);
        });
    }
    else {
        return convertInner(api, options);
    }
}

module.exports = {
    convert: convert,
    fakeBodyParameter: fakeBodyParameter
};
