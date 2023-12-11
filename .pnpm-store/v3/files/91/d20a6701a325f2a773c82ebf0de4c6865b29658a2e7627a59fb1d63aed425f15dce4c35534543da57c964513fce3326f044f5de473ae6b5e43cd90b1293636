'use strict';

var xml = '';
var hanging = '';
var followsElement = true;
var followsEndElement = false;
var depth = 0;
var pretty = 0;
var spacer = ' ';

function replaceAll(s, from, to) {
	return s.split(from).join(to);
}

function encode(s) {
	var es = s;
	if (typeof s === 'string') { // might be a number, boolean or null
		es = replaceAll(es, '&','&amp;');
		es = replaceAll(es, '<','&lt;');
		es = replaceAll(es, '>','&gt;');
		es = replaceAll(es, '"','&quot;');
		es = replaceAll(es, "'",'&apos;');
	}
	return es;
}

function startFragment(indent,indentStr) {
	if (indent) {
		pretty = indent;
		if ((indentStr) && (indentStr !== '')) {
			spacer = indentStr;
		}
		else {
			spacer = ' ';
		}
	}
	else {
		pretty = 0;
	}

	followsElement = true;
	followsEndElement = false;
	xml = '';
	hanging = '';
	depth = 0;
}

module.exports = {
	startFragment: startFragment,

	startDocument : function (encoding,standalone,indent,indentStr) {
		startFragment(indent,indentStr);
		xml = '<?xml version="1.0" encoding="' + (encoding ? encoding : 'UTF-8') + '"' +
		(standalone ? ' standalone="' + standalone + '"' : '') + ' ?>';
	},

	docType : function (s) {
		xml += '<!DOCTYPE ' + s + '>';
	},

	startElement : function (s) {
		xml += hanging;
		if (s !== '') {
			if ((pretty) && (followsElement || followsEndElement)) xml += '\n'+new Array(pretty*depth+1).join(spacer);
			xml += '<' + s;
			hanging = '>';
			depth++;
			followsElement = true;
		}
		else hanging = '';
	},

	emptyElement : function (s) {
		xml += hanging + '<' + s;
		hanging = '/>';
	},

	attribute : function (a,v) {
		xml += ' ' + a + '="' + encode(v) + '"';
	},

	content : function (s) {
		xml += hanging + encode(s);
		hanging = '';
		followsElement = false;
		followsEndElement = false;
	},

	comment : function (s) {
		xml += hanging + '<!--' + encode(s) + '-->';
		hanging = '';
	},

	processingInstruction : function (s) {
		xml += hanging;
		if ((pretty) && (followsElement || followsEndElement)) xml += '\n'+new Array(pretty*depth+1).join(spacer);
		xml += '<?' + encode(s) + '?>';
		hanging = '';
	},

	cdata : function (s) {
		xml += hanging;
		if ((pretty) && (followsElement || followsEndElement)) xml += '\n'+new Array(pretty*depth+1).join(spacer);
		xml += '<![CDATA[' + s + ']]>';
		hanging = '';
	},

	fragment : function (s) {
		xml += hanging + s;
		hanging = '';
		followsElement = false;
		followsEndElement = true;
	},

	endElement : function (s) {
		var suppress = false;
		if (hanging == '>') {
			hanging = '/>';
			suppress = true;
		}
		xml += hanging;
		if (s !== '') {
			depth--;
			if (!suppress) {
				if (pretty && followsEndElement) xml += '\n'+new Array(pretty*depth+1).join(spacer);
				xml += '</' + s + '>';
			}
			followsElement = false;
			followsEndElement = true;
		}
		hanging = '';
	},

	endFragment : function () {
		xml += hanging;
		hanging = '';
		return xml;
	},

	endDocument : function () {
		//hanging is '' at this point
		return xml;
	}
};
