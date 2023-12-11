/*

The Just-Good-Enough XML Parser

*/

'use strict';

const sInitial = 0;
const sDeclaration = 1;
const sPreElement = 2;
const sElement = 3;
const sAttribute = 5;
const sAttrNML = 6; // No Mans Land
const sValue = 7;
const sEndElement = 9;
const sContent = 11;
const sAttributeSpacer = 12;
const sComment = 13;
const sProcessingInstruction = 15;
const sCData = 17;
const sDocType = 19;
const sDTD = 21;
const sError = 23;
const sEndDocument = 25;

require('./common');

function stateName(state) {
	if (state == sInitial) {
		return 'INITIAL';
	}
	else if (state == sDeclaration) {
		return 'DECLARATION';
	}
	else if (state == sElement) {
		return 'ELEMENT';
	}
	else if (state == sAttribute) {
		return 'ATTRIBUTE';
	}
	else if (state == sValue) {
		return 'VALUE';
	}
	else if (state == sEndElement) {
		return 'END_ELEMENT';
	}
	else if (state == sContent) {
		return 'CONTENT';
	}
	else if (state == sComment) {
		return 'COMMENT';
	}
	else if (state == sProcessingInstruction) {
		return 'PROCESSING_INSTRUCTION';
	}
	else if (state == sCData) {
		return 'CDATA';
	}
	else if (state == sDocType) {
		return 'DOCTYPE';
	}
	else if (state == sDTD) {
		return 'DTD';
	}
	else if (state == sError) {
		return 'ERROR';
	}
	else if (state == sEndDocument) {
		return 'END_DOCUMENT';
	}
}

function reset(context) {
	context.state = sInitial;
	context.newState = sInitial;
	context.token = '';
	context.boundary = ['<?','<'];
	context.bIndex = -1;
	context.lastElement = '';
	context.keepToken = false;
	context.position = 0;
	context.depth = 0;
	context.wellFormed = false;
	context.validControlChars = ['\t','\r','\n'];
	context.error = false;
}

// to create a push parser, pass in a callback function and omit the context parameter
// to create a pull parser, pass in null for the callback function and initially provide an empty object as the context
function jgeParse(s,callback,context) {

	if (context && context.newState) {
		if (!context.keepToken) context.token = '';
		context.state = context.newState;
	}
	else {
		context = {};
		reset(context);
	}

	var c;
	for (var i=context.position;i<s.length;i++) {
		c = s.charAt(i);
		if ((c.charCodeAt(0) < 32) && (context.validControlChars.indexOf(c) < 0)) {
			context.newState = context.state = sError;
		}

		if (context.state != sContent) {
			if (context.validControlChars.indexOf(c) >= 0) { //other unicode spaces are not treated as whitespace
				c = ' ';
			}
		}

		context.bIndex = -1;
		for (var b=0;b<context.boundary.length;b++) {
			if (s.substr(i,context.boundary[b].length) == context.boundary[b]) {
				context.bIndex = b;
				if (context.boundary[context.bIndex].length>1) {
					i = i + context.boundary[context.bIndex].length-1;
				}
				break;
			}
		}

		if (context.bIndex >= 0) {

			if ((context.state != sValue) && (context.state != sComment)) { // && (context.state != sContent)
				context.token = context.token.trim();
			}

			context.keepToken = false;
			if (((context.state & 1) == 1) && ((context.token.trim() !== '') || context.state == sValue)) {
				// TODO test element names for validity (using regex?)
				if (context.state != sCData) {
					context.token = context.token.replaceAll('&amp;','&');
					context.token = context.token.replaceAll('&quot;','"');
					context.token = context.token.replaceAll('&apos;',"'");
					context.token = context.token.replaceAll('&gt;','>');
					context.token = context.token.replaceAll('&lt;','<');
					if (context.token.indexOf('&#') >= 0) {
						context.token = context.token.replace(/&(?:#([0-9]+)|#x([0-9a-fA-F]+));/g, function(match, group1, group2) {
							var e;
							if (group2) {
								e = String.fromCharCode(parseInt(group2,16));
								if ((e.charCodeAt(0) < 32) && (context.validControlChars.indexOf(e) < 0)) {
									context.newState = context.state = sError;
								}
								return e;
							}
							else {
								e = String.fromCharCode(group1);
								if ((e.charCodeAt(0) < 32) && (context.validControlChars.indexOf(e) < 0)) {
									context.newState = context.state = sError;
								}
								return e;
							}
						});
					}
				}

				if (context.state == sElement) context.depth++;
				else if (context.state == sEndElement) {
					context.depth--;
					if (context.depth<0) {
						context.newState = context.state = sError;
					}
				}
				if (context.state == sError) {
					context.error = true;
				}
				if (callback) {
					callback(context.state,context.token);
				}
				if (context.state == sError) {
					context.boundary = [];
				}
			}

			if (context.state == sInitial) {
				if (context.boundary[context.bIndex] == '<?') {
					context.newState = sDeclaration;
					context.boundary = ['?>'];
				}
				else {
					context.newState = sElement;
					context.boundary = ['>',' ','/','!--','?','!DOCTYPE','![CDATA['];
					context.boundary = context.boundary.concat(context.validControlChars);
				}
			}
			else if (context.state == sDeclaration) {
				context.newState = sPreElement;
				context.boundary = ['<'];
				if (context.token.indexOf('1.1')>0) {
					context.validControlChars.push('\u2028','\u0085','\u0015');
				}
			}
			else if (context.state == sPreElement) {
				context.newState = sElement;
				context.boundary = ['>',' ','/','!--','?','!DOCTYPE','![CDATA['];
				context.boundary = context.boundary.concat(context.validControlChars);
			}
			else if (context.state == sElement) {
				context.lastElement = context.token;
				if (c == '>') {
					context.newState = sContent;
					context.boundary = ['<!DOCTYPE','<'];
				}
				else if (c == ' ') {
					context.newState = sAttribute;
					context.boundary = ['/','=','>'];
				}
				else if (c == '/') {
					context.newState = sEndElement;
					context.boundary = ['>'];
					context.keepToken = true;
				}
				else if (c == '?') {
					context.newState = sProcessingInstruction;
					context.boundary = ['?>'];
				}
				else if (context.boundary[context.bIndex] == '!--') {
					context.newState = sComment;
					context.boundary = ['-->'];
				}
				else if (context.boundary[context.bIndex] == '![CDATA[') {
					context.newState = sCData;
					context.boundary = [']]>'];
				}
				else if (context.boundary[context.bIndex] == '!DOCTYPE') {
					context.newState = sDocType;
					context.boundary = ['>','['];
				}
			}
			else if (context.state == sAttribute) {
				if (c == '=' ) {
					context.newState = sAttrNML;
					context.boundary = ['\'','"'];
				}
				else if (c == '>') {
					context.newState = sContent;
					context.boundary = ['<!DOCTYPE','<'];
				}
				else if (c == '/') {
					context.newState = sEndElement;
					context.keepToken = true;
					context.state = sAttributeSpacer; // to stop dummy attributes being emitted to pullparser
					context.token = context.lastElement;
				}
			}
			else if (context.state == sAttrNML) {
				context.newState = sValue;
				context.boundary = [c];
			}
			else if (context.state == sValue) {
				context.newState = sAttribute;
				context.boundary = ['=','/','>'];
			}
			else if (context.state == sEndElement) {
				if (context.depth !== 0) context.newState = sContent;
				context.boundary = ['<']; // don't allow DOCTYPE's after the first sEndElement
			}
			else if (context.state == sContent) {
				if (context.boundary[context.bIndex] == '<!DOCTYPE') {
					context.newState = sDocType;
					context.boundary = ['>','['];
				}
				else {
					context.newState = sElement;
					context.boundary = ['>',' ','/','!--','?','![CDATA['];
					context.boundary = context.boundary.concat(context.validControlChars);
				}
			}
			else if (context.state == sComment) {
				context.newState = sContent;
				context.boundary = ['<!DOCTYPE','<'];
			}
			else if (context.state == sProcessingInstruction) {
				context.newState = sContent;
				context.boundary = ['<!DOCTYPE','<'];
			}
			else if (context.state == sCData) {
				context.newState = sContent;
				context.boundary = ['<!DOCTYPE','<'];
			}
			else if (context.state == sDocType) {
				if (context.boundary[context.bIndex] == '[') {
					context.newState = sDTD;
					context.boundary = [']>'];
				}
				else {
					context.newState = sPreElement;
					context.boundary = ['<'];
				}
			}
			else if (context.state == sDTD) {
				context.newState = sPreElement;
				context.boundary = ['<'];
			}

			if (!callback) {
				if (((context.state & 1) == 1) && ((context.token.trim() !== '') || context.state == sValue)) {
					context.position = i+1;
					return context;
				}
			}
			context.state = context.newState;

			if (!context.keepToken) context.token = '';
		}
		else {
			context.token += c;
		}

	}
	if ((context.state == sEndElement) && (context.depth === 0) && (context.token.trim() === '')) {
		context.wellFormed = true;
	}
	if ((!context.wellFormed) && (!context.error)) {
		if (callback) {
			// generate a final error, only for pushparsers though
			callback(sError,context.token);
		}
	}
	context.state = sEndDocument;
	if (callback) {
		callback(context.state,context.token);
		return context.wellFormed;
	}
	else {
		return context;
	}
}

module.exports = {
	parse : jgeParse,
	getStateName : stateName,
	sInitial : sInitial,
	sDeclaration : sDeclaration,
	sElement : sElement,
	sAttribute : sAttribute,
	sValue : sValue,
	sEndElement : sEndElement,
	sContent : sContent,
	sComment : sComment,
	sProcessingInstruction: sProcessingInstruction,
	sCData : sCData,
	sDocType : sDocType,
	sDTD : sDTD,
	sEndDocument : sEndDocument
};
