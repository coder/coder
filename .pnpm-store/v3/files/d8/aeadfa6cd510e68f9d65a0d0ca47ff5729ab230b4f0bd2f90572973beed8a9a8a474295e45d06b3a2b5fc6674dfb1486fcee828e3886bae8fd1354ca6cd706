'use strict';

var xw = require('../xmlWrite');

xw.startFragment(2);
xw.docType('fubar');
xw.startElement('foo');
xw.processingInstruction('nitfol xyzzy');
xw.startElement('bar');
xw.comment('potrzebie');
xw.content('baz');
xw.cdata('snafu');
xw.endElement('bar');
xw.endElement('foo');

console.log(xw.endFragment());
