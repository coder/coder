'use strict';

function generate(data) {
    const effectiveBaseUrl = (data.baseUrl == '//') ? 'https://example.com' : data.baseUrl;
    const url = data.requiredUriExample ? `${effectiveBaseUrl}${data.requiredUriExample}` : data.url;

    const request = {
        httpVersion: 'HTTP/1.1',
        method: data.methodUpper,
        url,
        queryString: getValues(data.queryParameters),
        headers: getValues(data.allHeaders),
        headersSize: -1,
        cookies: []
    };

    if (data.bodyParameter && data.bodyParameter.present) {
        request.bodySize = -1;
        request.postData = {
            mimeType: data.bodyParameter.contentType
        };

        if (request.postData.mimeType === 'multipart/form-data') {
            request.postData.params = Object.keys(data.bodyParameter.exampleValues.object)
                .map(field => {
                    return { name: field, value: data.bodyParameter.exampleValues.object[field] };
                });
        }
        else {
            request.postData.text = JSON.stringify(data.bodyParameter.exampleValues.object);
        }
    }

    return request;
}

function getValues(items) {
    return (items || [])
        .map((item) => {
            return { name: item.name, value: item.exampleValues.object.toString() };
        });
}

module.exports = {
    generate
};
