'use strict';

const assert = require('assert');
const harGenerator = require('../lib/harGenerator');

const sampleData = {
    methodUpper: 'GET',
    baseUrl: 'http://sample.com',
    requiredUriExample: '/books/1',
    url: 'http://sample.com/books/{id}',
    queryParameters: [],
    allHeaders: []
};

const sampleHeaders = [
    { name: 'Accept', exampleValues: { object: 'application/json' } }
];

const sampleQueryStringParams = [
    { name: 'param1', exampleValues: { object: 'value1' } }
];

const baseRequest = {
    httpVersion: 'HTTP/1.1',
    method: 'GET',
    url: 'http://sample.com/books/1',
    queryString: [],
    headers: [],
    headersSize: -1,
    cookies: []
}

describe('harGenerator tests', () => {
    it('should generate request for a url', () => {
        const testData = Object.assign({}, sampleData);
        const expected = baseRequest;

        const result = harGenerator.generate(testData);

        assert.deepEqual(result, expected);
    });

    it('should include specified headers in generated request', () => {
        const testData = Object.assign({}, sampleData, { allHeaders: sampleHeaders });
        const expected = Object.assign({}, baseRequest, { headers: [{ name: 'Accept', value: 'application/json' }] });

        const result = harGenerator.generate(testData);

        assert.deepEqual(result, expected);
    });

    it('should include specified query string parameters in generated request', () => {
        const testData = Object.assign({}, sampleData, { queryParameters: sampleQueryStringParams });
        const expected = Object.assign({}, baseRequest, { queryString: [{ name: 'param1', value: 'value1' }] });

        const result = harGenerator.generate(testData);

        assert.deepEqual(result, expected);
    });

    it('should include body payload in generated request, if present', () => {
        const bodyParameter = {
            present: true,
            contentType: 'application/json',
            exampleValues: {
                object: { a: 10 },
                json: JSON.stringify({ a: 10 }, true, 2)
            }
        };
        const testData = Object.assign({}, sampleData, { methodUpper: 'POST', bodyParameter });
        const expected = Object.assign(
            {},
            baseRequest,
            {
                method: 'POST',
                bodySize: -1,
                postData: { mimeType: 'application/json', text: '{\"a\":10}' }
            }
        );

        const result = harGenerator.generate(testData);

        assert.deepEqual(result, expected);
    });

    it('should use postData.params in case of multipart/form-data payload', () => {
        const bodyParameter = {
            present: true,
            contentType: 'multipart/form-data',
            exampleValues: {
                object: { a: 10 },
                json: JSON.stringify({ a: 10 }, true, 2)
            }
        };
        const testData = Object.assign({}, sampleData, { methodUpper: 'POST', bodyParameter });
        const expected = Object.assign(
            {},
            baseRequest,
            {
                method: 'POST',
                bodySize: -1,
                postData: { mimeType: 'multipart/form-data', params: [{ name: "a", value: 10 }] }
            }
        );

        const result = harGenerator.generate(testData);

        assert.deepEqual(result, expected);
    });

    it('should use operation url if uri example is not present', () => {
        const testData = Object.assign({}, sampleData, { requiredUriExample: null });
        const expected = Object.assign({}, baseRequest, { url: 'http://sample.com/books/{id}' });

        const result = harGenerator.generate(testData);

        assert.deepEqual(result, expected);
    });
});
