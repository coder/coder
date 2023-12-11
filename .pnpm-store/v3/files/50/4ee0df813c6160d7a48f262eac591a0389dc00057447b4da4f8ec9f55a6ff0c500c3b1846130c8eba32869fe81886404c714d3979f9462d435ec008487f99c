'use strict';

const openapi2 = require('./openapi2.js');
const openapi3 = require('./openapi3.js');
const asyncapi1 = require('./asyncapi1.js');
const semoasa = require('./semoasa.js');
const apiblueprint = require('./apiblueprint.js');

function convert(api, options) {

    options.samplerErrors = new Map();

    if (typeof api === 'string') {
        return apiblueprint.convert(api, options);
    }
    else if (api.swagger) {
        return openapi2.convert(api, options);
    }
    else if (api.openapi) {
        return openapi3.convert(api, options);
    }
    else if (api.asyncapi) {
        return asyncapi1.convert(api, options);
    }
    else if (api.openapiExtensionFormat) {
        return semoasa.convert(api, options);
    }
    else {
        throw new Error('Unrecognised input format');
    }
}

module.exports = {
    convert: convert
};
