'use strict';

var fs = require('fs');
var stream = require('stream');
var crypto = require('crypto');

var jgeXml = require('./jgeXml');
var xw = require('./xmlWrite');
var x2j = require('./xml2json');
var j2x = require('./json2xml');
var xsd2j = require('./xsd2json');
var jpath = require('./jpath');

var passing = 0;
var failing = 0;
var encoding = 'utf'; // nodejs input encoding, not XML encoding
var valueProperty = false;
var coerceTypes = false;

function lines(s) {
	return s.split('\n');
}

function diff(s1,s2) {
	var red = process.env.NODE_DISABLE_COLORS ? '' : '\x1b[31m';
	var green = process.env.NODE_DISABLE_COLORS ? '' : '\x1b[32m';
	var normal = process.env.NODE_DISABLE_COLORS ? '' : '\x1b[0m';
	var l1 = lines(s1);
	var l2 = lines(s2);
	var top = l1.length > l2.length ? l2.length : l1.length;
	for (var l=0;l<top;l++) {
		if (l1[l] != l2[l]) {

			console.log('Line '+(l+1));
			var cs = Math.max(0,l-3);
			for (var c=cs;c<l;c++) {
				console.log('  '+l1[c]);
			}
			console.log('- '+red+l2[l]+normal);
			console.log('+ '+green+l1[l]+normal);
			cs = Math.min(top,l+4);
			for (c=l+1;c<cs;c++) {
				console.log('  '+((l1[c] == l2[c]) ? green : red)+l1[c]+normal);
			}
			break;
		}
	}
}

function runXmlTest(filename,components) {
	var stem = '';
	for (var c=0;c<components.length-1;c++) {
		stem += (stem ? '.' : '') + components[c];
	}

	var exists = false;
	try {
		fs.statSync('out/'+stem+'.json',fs.R_OK);
		exists = true;
	}
	catch(err) {}

	if (exists) {
		console.log('  Convert and compare to JSON');
		var xml = fs.readFileSync(testdir+'/'+filename,encoding);
		var attrPrefix = '@';
		if (filename.indexOf('noap')>=0) {
			attrPrefix = '';
		}
		var obj = x2j.xml2json(xml,{"attributePrefix": attrPrefix, "valueProperty": valueProperty, "coerceTypes": coerceTypes});
		var json = JSON.stringify(obj,null,2);
		var compare = fs.readFileSync('out/'+stem+'.json',encoding);
		compare = compare.replaceAll('\r\n','\n');

		if (json.trim() == compare.trim()) {
			passing++;
		}
		else {
			diff(json,compare);
			console.log('  Fail');
			failing++;
		}

		exists = false;
		try {
			fs.statSync('out/'+stem+'.jpath',fs.R_OK);
			exists = true;
		}
		catch(err) {}

		if (exists) {
			console.log('  Test JSONPath');
			var jpt	= fs.readFileSync('out/'+stem+'.jpath',encoding);
			var jpo = JSON.parse(jpt);

			var tree = jpath.build(obj);
			var success = true;

			for (var j in jpo) {
				var test = jpo[j];

				var query = test.query;
				var fetch = test.fetch;
				var expected = test.expected;

				var output = jpath.select(tree,query)[0].value;

				if ((expected != output) || (expected != jpath.fetchFromObject(obj,fetch))) {
					console.log(output);
					console.log(jpath.fetchFromObject(obj,fetch));
					console.log(expected);
					success = false;
				}

			}
			if (!success) {
				console.log('  Fail jpath');
				failing++;
			}
		}

	}
}

function runXsdTest(filename,components) {
	var stem = '';
	for (var c=0;c<components.length-1;c++) {
		stem += (stem ? '.' : '') + components[c];
	}

	var exists = false;
	try {
		fs.statSync('out/'+stem+'.json',fs.R_OK);
		exists = true;
	}
	catch(err) {}

	if (exists) {
		console.log('  Convert and compare to JSON');
		var xml = fs.readFileSync(testdir+'/'+filename,encoding);
		var j1 = x2j.xml2json(xml,{"attributePrefix": "@", "valueProperty": valueProperty, "coerceTypes": coerceTypes});
		var laxUris = (filename.indexOf('.lax')>=0);
		var obj = xsd2j.getJsonSchema(j1,testdir+'/'+filename,'',laxUris);
		var json = JSON.stringify(obj,null,2);
		var compare = fs.readFileSync('out/'+stem+'.json',encoding);
		compare = compare.replaceAll('\r\n','\n');

		if (json.trim() == compare.trim()) {
			passing++;
		}
		else {
			diff(json,compare);
			console.log('  Fail');
			failing++;
		}
	}
}

function runJsonTest(filename,components) {
	var stem = '';
	for (var c=0;c<components.length-1;c++) {
		stem += (stem ? '.' : '') + components[c];
	}

	var	exists = false;
	try {
		fs.statSync('out/'+stem+'.xml',fs.R_OK);
		exists = true;
	}
	catch(err) {}

	if (exists) {
		console.log('  Convert and compare to XML');
		var json = fs.readFileSync(testdir+'/'+filename,encoding);
		var obj = JSON.parse(json);
		var xml = j2x.getXml(obj,'@','',2);
		var compare = fs.readFileSync('out/'+stem+'.xml',encoding);
		compare = compare.replaceAll('\r\n','\n');

		if (xml == compare) {
			passing++;
		}
		else {
			diff(xml,compare);
			console.log('  Fail');
			failing++;
		}
	}
}

function testXml(filename,components,expected) {
	if (!expected) console.log('  Expected to fail');
	var xml = fs.readFileSync(testdir+'/'+filename,encoding);
	var ok = true;
	var result = jgeXml.parse(xml,function(state,token) {
		var stateName = jgeXml.getStateName(state);
		if (stateName == 'ERROR') {
			ok = false;
		}
	});
	if ((result == expected) && (ok == expected)) {
		passing++;
	}
	else {
		console.log('  Error');
		failing++;
	}
}

function testXmlPull(filename,components,expected) {
	if (!expected) console.log('  Expected to fail');
	var xml = fs.readFileSync(testdir+'/'+filename,encoding);
	var ok = true;
	var context = {};
	while ((!context.state) || (context.state != jgeXml.sEndDocument)) {
		context = jgeXml.parse(xml,null,context);
		var stateName = jgeXml.getStateName(context.state);
		if (stateName == 'ERROR') {
			ok = false;
		}
	}
	var result = context.wellFormed;
	if ((result == expected) && (ok == expected)) {
		passing++;
	}
	else {
		console.log('  Error');
		failing++;
	}
}

process.exitCode = 1; // in case of crash

var testdir = 'test';
if (process.argv.length>2) testdir = process.argv[2];

var xmlTypes = ['xml','xsl','xhtml','svg','wsdl','config', 'mpd'];

var tests = fs.readdirSync(testdir);
for (var t in tests) {
	var filename = tests[t];
	console.log(filename);
	var components = filename.split('.');

	encoding = 'utf8';
	if (components.indexOf('utf16') >= 0) encoding = 'ucs2';

	valueProperty = false;
	if (components.indexOf('valueProperty') >= 0) valueProperty = true;

	coerceTypes = false;
	if (components.indexOf('coerceTypes') >= 0) coerceTypes = true;

	if ((xmlTypes.indexOf(components[components.length-1]) >= 0) && (components.indexOf('invalid') >= 0)) {
		testXml(filename,components,false);
	}
	else if (xmlTypes.indexOf(components[components.length-1]) >= 0) {
		if (components.indexOf('pull') >= 0) {
			testXmlPull(filename,components,true);
		}
		else {
			testXml(filename,components,true);
		}
		runXmlTest(filename,components);
	}
	else if (components[components.length-1] == 'xsd') {
		testXml(filename,components,true);
		runXsdTest(filename,components);
	}
	else if (components[components.length-1] == 'json') {
		runJsonTest(filename,components);
	}
}

var frag1 = `<!DOCTYPE fubar>
<foo>
  <?nitfol xyzzy?>
  <bar><!--potrzebie-->baz<![CDATA[snafu]]></bar>
</foo>`;
var xml = fs.readFileSync('out/fragment1.xml',encoding);
xml = xml.replaceAll('\r','');
if (frag1 == xml) {
	passing++;
}
else {
	diff(frag1,xml);
	failing++;
}

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
frag1 = xw.endFragment();
if (frag1 == xml) {
	passing++;
}
else {
	diff(frag1,xml);
	failing++;
}

console.log(passing + ' passing, ' + failing + ' failing');
process.exitCode = (failing === 0) ? 0 : 1;
