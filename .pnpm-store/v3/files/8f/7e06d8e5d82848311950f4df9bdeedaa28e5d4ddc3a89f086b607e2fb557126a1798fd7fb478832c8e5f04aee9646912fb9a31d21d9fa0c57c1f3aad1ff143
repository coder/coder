'use strict';

var fs = require('fs');
var jgeXml = require('../jgeXml.js');

var filename = process.argv[2];
if (!filename) {
	console.warn('Usage: pushparser {infile} [encoding]');
	process.exit(1);
}
var encoding = 'utf8';
if (process.argv.length>3) encoding = process.argv[3];

var xml = fs.readFileSync(filename,encoding);
console.log(xml);
console.log();

var depth = 0;

var result = jgeXml.parse(xml,function(state,token){
	if (state == jgeXml.sElement) {
		depth++;
	}
	else if (state == jgeXml.sEndElement) {
		depth--;
	}
	console.log(jgeXml.getStateName(state)+' '+depth+' "'+token+'"');
});
console.log(result);
if (!result) process.exitCode = 1;
