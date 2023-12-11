'use strict';

var fs = require('fs');
var path = require('path');
var x2j = require('../xml2json');
var jpath = require('../jpath');

function dumpResults(tree,query) {
	console.log();
	console.log(query);
	var matches = jpath.select(tree,query);
	for (var m in matches) {
		console.log(matches[m].value ? matches[m].value : matches[m]);
	}
	return matches;
}

function testBookStore(tree) {
	console.log();
	console.log('That looks like a bookstore!');

	dumpResults(tree,'$..*');
	dumpResults(tree,'*');
	dumpResults(tree,'$.store.book[*].author');
	dumpResults(tree,'$..author');
	dumpResults(tree,'$.store.*');
	dumpResults(tree,'$.store..price');
	dumpResults(tree,'$..book[2]');
	dumpResults(tree,'$..book[2].price');
	dumpResults(tree,'$..bicycle.price');
	dumpResults(tree,'$..bicycle.price^');
}

function propTest(obj,path) {
	console.log(path+' = '+jpath.fetchFromObject(obj,path))
}

function ptrTest(obj,path) {
	console.log(path+' = '+jpath.jptr(obj,path))
}

function testProperties(obj) {
	console.log();
	propTest(obj,'store.book[2].price');
	ptrTest(obj,'/store/book/2/price');
	jpath.jptr(obj,'/store/book/2/price',10.99);
	ptrTest(obj,'/store/book/2/price');
}

var filename = process.argv[2] || path.resolve(__dirname,'../test/bookstore.json');
var valueProperty = false;
if (process.argv.length>3) {
	valueProperty = true;
}

var xml = fs.readFileSync(filename,'utf8');

try {
	var obj = x2j.xml2json(xml,{"attributePrefix": "@","valueProperty": valueProperty, "coerceTypes": false});
}
catch (err) {
	console.error('That is not valid JSON');
	console.error(err);
	console.log(xml);
	console.log();
	console.log(x2j.getString());
	process.exit(1);
}

var tree = jpath.build(obj);
// we could do a select('*') here but it's redundant unless we want the bracketed form
for (var i in tree) {
	// log our jpath for every item
	console.log(tree[i].depth+' '+jpath.path(tree[i],false)+' = '+tree[i].value);
}

if (tree.length>0) {
	if (obj.store && obj.store.book) {
		testBookStore(tree);
		testProperties(obj);
	}
	else {
		// generic examples
		var first = jpath.path(tree[1]);
		var matches = jpath.select(tree,first);
		for (var m in matches) {
			console.log('First; select('+jpath.path(matches[m])+') = '+matches[m].value);
		}

		var last = tree[tree.length-1];
		console.log('Last; select('+jpath.path(last,true)+',true) = '+last.value);
		console.log(last.prefix)

		var parents = jpath.select(tree,last.prefix);
		if (parents.length>0) {
			var value = parents[0].value;
			if (typeof(value) === 'object') {
				value = JSON.stringify(value,null,2);
			}
			console.log('select('+jpath.path(parents[0],true)+',true) = '+value);
		}
	}
}
