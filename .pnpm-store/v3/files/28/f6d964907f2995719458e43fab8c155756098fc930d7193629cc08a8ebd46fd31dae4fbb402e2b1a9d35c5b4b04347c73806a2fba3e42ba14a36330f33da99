'use strict';

var fs = require('fs');
var jgeXml = require('../jgeXml.js');
var xmlWrite = require('../xmlWrite.js');

function x2x(xml) {
	var attributeName = '';

	jgeXml.parse(xml,function(state,token){

		if (state == jgeXml.sDeclaration) {
			xmlWrite.startDocument('UTF-8','',2);
		}
		else if (state == jgeXml.sComment) {
			xmlWrite.comment(token);
		}
		else if (state == jgeXml.sProcessingInstruction) {
			xmlWrite.processingInstruction(token);
		}
		else if (state == jgeXml.sCData) {
			xmlWrite.cdata(token);
		}
		else if (state == jgeXml.sContent) {
			xmlWrite.content(token);
		}
		else if (state == jgeXml.sEndElement) {
			xmlWrite.endElement(token);
		}
		else if (state == jgeXml.sAttribute) {
			attributeName = token;
		}
		else if (state == jgeXml.sValue) {
			xmlWrite.attribute(attributeName,token);
		}
		else if (state == jgeXml.sElement) {
			xmlWrite.startElement(token);
		}
	});
	return xmlWrite.endDocument();
}

var filename = process.argv[2];
if (!filename) {
	console.warn('Usage: xml2xml {infile}');
	process.exit(1);
}

var xml = fs.readFileSync(filename,'utf8');

var s1 = x2x(xml); // normalise declaration, spacing and empty elements etc
var s2 = x2x(s1); // compare
var same = (s1 == s2);
if (!same) {
	console.warn(s1);
	process.exitCode = 1;
}
console.log(s2);
