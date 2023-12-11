'use strict';

const openapi3 = require('./openapi3.js');
const swagger2openapi = require('swagger2openapi');

function convert(api, options) {
    return swagger2openapi.convertObj(api, { patch: true, anchors: true, warnOnly: true, resolve: options.resolve, verbose: options.verbose, source: options.source, rbname: options.useBodyName ? 'x-body-name' : '' })
        .then(sOptions => {
            options.resolve = false; // done now
            return openapi3.convert(sOptions.openapi, options);
        })
        .catch(err => {
            if (options.verbose) {
                console.error(err);
            }
            else {
                console.error(err.message);
            }
            throw err;
        });
}

module.exports = {
    convert: convert
};
