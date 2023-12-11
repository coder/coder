#!/usr/bin/env node
'use strict';

var fs = require('fs');
var x2j = require('../xml2json');

var filename = process.argv[2];
if (!filename) {
	console.warn('Usage: xml2json {infile}');
	process.exit(1);
}
var valueProperty = false;
var coerceTypes = false;

if (process.argv.length>3) {
	valueProperty = (process.argv[3] != '0');
}
if (process.argv.length>4) {
	coerceTypes = (process.argv[4] != '0');
}

var xml = fs.readFileSync(filename,'utf8');

var obj = x2j.xml2json(xml,{"attributePrefix": "@","valueProperty": valueProperty, "coerceTypes": coerceTypes});
console.log(JSON.stringify(obj,null,2));
