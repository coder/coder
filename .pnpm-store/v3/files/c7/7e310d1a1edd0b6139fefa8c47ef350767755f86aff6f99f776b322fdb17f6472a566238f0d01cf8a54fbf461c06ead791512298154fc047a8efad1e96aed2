'use strict';

var fs = require('fs');
var jgeXml = require('../jgeXml.js');

var filename = process.argv[2];
if (!filename) {
	console.warn('Usage: pullparser {infile} [encoding]');
	process.exit(1);
}
var encoding = 'utf8';
if (process.argv.length>3) encoding = process.argv[3];

var xml = fs.readFileSync(filename,encoding);
console.log(xml);
console.log();

var context = {};
var depth = 0;

while (!context.state || context.state != jgeXml.sEndDocument) {
	context = jgeXml.parse(xml,null,context);
	if (context.state == jgeXml.sElement) {
		depth++;
	}
	else if (context.state == jgeXml.sEndElement) {
		depth--;
	}
	console.log(jgeXml.getStateName(context.state)+' '+context.position+' '+depth+' '+context.depth+' "'+context.token+'"');
}
console.log(context.wellFormed);
if (!context.wellFormed) process.exitCode = 1;
