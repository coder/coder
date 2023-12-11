'use strict';

const assert = require('assert');
const openapi3 = require('../lib/openapi3');
const options =
    {
        'omitBody': true,
        'expandBody': false
    };
const operation =
    {
        'requestBody': {
            'required': false,
            'description': 'This is a description'
        }
    };
const bodyParameter =
    {
        'refName': 'name of reference',
        'schema': {
            'type': 'object',
            'properties':
                {
                    'id':
                        {
                            'type': 'string',
                            'description': 'The fake ID'
                        }
                }
        }
    };

const noOptions = {
    'options': {},
    'operation': operation,
    'parameters': [],
    'bodyParameter': bodyParameter,
    'translations': {
        'indent': ''
    }
};

const noOperation = {
    'options': options,
    'operation': {},
    'parameters': [],
    'bodyParameter': bodyParameter,
    'translations': {
        'indent': ''
    }
};

const goodData =
    {
        'options': options,
        'operation': operation,
        'parameters': [],
        'bodyParameter': bodyParameter,
        'translations': {
            'indent': ''
        }
    };

describe('openapi3 tests', () => {
    describe('fakeBodyParameter', () => {
        it('should handle empty options', () => {
            assert.doesNotThrow(() => openapi3.fakeBodyParameter(noOptions));
        });

        it('should handle empty operation', () => {
            assert.doesNotThrow(() => openapi3.fakeBodyParameter(noOperation));
        });

        it('should append parameters to data.parameters', () => {
            openapi3.fakeBodyParameter(goodData);
            assert(goodData.parameters.length === 1);
        });

    });
});
