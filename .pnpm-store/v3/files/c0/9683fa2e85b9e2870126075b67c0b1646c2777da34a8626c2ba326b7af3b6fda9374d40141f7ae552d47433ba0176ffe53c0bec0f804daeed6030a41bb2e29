#!/usr/bin/env node
'use strict';

var fs = require('fs');
var x2j = require('../xml2json');
var xsd = require('../xsd2json');

var filename = process.argv[2];
if (!filename) {
	console.warn('Usage: xsd2json {infile} [{outfile}]');
	process.exit(1);
}
var valueProperty = false;

var xml = fs.readFileSync(filename,'utf8');

try {
	var obj = x2j.xml2json(xml,{"attributePrefix": "@","valueProperty": valueProperty, "coerceTypes": false});
}
catch (err) {
	console.error('That is not valid JSON');
	console.error(err);
	console.log(x2j.getString());
	process.exit(1);
}

var laxUris = (filename.indexOf('.lax')>=0);
var json = xsd.getJsonSchema(obj,filename,'',laxUris,'xs:');

if (process.argv.length>3) {
	var outfile = process.argv[3];
	fs.writeFileSync(outfile,JSON.stringify(json,null,2),'utf8');
}
else {
	console.log(JSON.stringify(json,null,2));
}
