'use strict';

var xmlWrite = require('./xmlWrite');

function traverse(obj,parent,attributePrefix) {

var result = [];

	var array = Array.isArray(obj);
	for (var key in obj) {
		// skip loop if the property is from prototype
		if (!obj.hasOwnProperty(key)) continue;

		var propArray = Array.isArray(obj[key]);
		var output = array ? parent : key;

		if (typeof obj[key] !== 'object'){
			if (key.indexOf(attributePrefix) === 0) {
				xmlWrite.attribute(key.substring(1),obj[key]);
			}
			else {
				xmlWrite.startElement(output);
				xmlWrite.content(obj[key]);
				xmlWrite.endElement(output);
			}
		}
		else {
			if (!propArray) {
				xmlWrite.startElement(output);
			}
			traverse(obj[key],output,attributePrefix);
			if (!propArray) {
				xmlWrite.endElement(output);
			}
		}
	}
	return result;
}

module.exports = {
	// TODO convert this to an options object
	getXml : function(obj,attrPrefix,standalone,indent,indentStr,fragment) {
		var attributePrefix = (attrPrefix ? attrPrefix : '@');
		if (fragment) {
			xmlWrite.startFragment(indent,indentStr);
		}
		else {
			xmlWrite.startDocument('UTF-8',standalone,indent,indentStr);
		}
		traverse(obj,'',attributePrefix);
		return xmlWrite.endDocument();
	}
};