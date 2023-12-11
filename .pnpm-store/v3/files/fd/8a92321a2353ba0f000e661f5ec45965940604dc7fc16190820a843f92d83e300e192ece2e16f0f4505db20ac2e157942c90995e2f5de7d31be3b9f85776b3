'use strict';

function convert(api, options) {
    return new Promise(function (resolve, reject) {
        api = api.split('\r').join('');
        var lines = api.split('\n');
        var title = '';
        var metadata = [];
        var index = 0;
        while ((lines[index].indexOf(':') >= 0) && (index < lines.length)) {
            metadata.push('> ' + lines[index] + '\n');
            lines[index] = '';
            index++;
        }
        while (lines[index] && !lines[index].startsWith('# ') && !lines[index].startsWith('==') && (index < lines.length)) {
            index++;
        }
        if (lines[index].startsWith('# ')) {
            title = lines[index];
        }
        else {
            title = lines[index - 1];
        }
        lines.splice(index + 1, 0, ...metadata);
        api = lines.join('\n');
        lines = [];
        api = '\n' + api + '\n';
        api = '---\ntitle: ' + (title.replace('# ', '')) + '\n' + `language_tabs:
toc_footers: []
includes: []
search: true
highlight_theme: darkula
---
` + api;
        resolve(api);
    });
}

module.exports = {
    convert: convert
};
