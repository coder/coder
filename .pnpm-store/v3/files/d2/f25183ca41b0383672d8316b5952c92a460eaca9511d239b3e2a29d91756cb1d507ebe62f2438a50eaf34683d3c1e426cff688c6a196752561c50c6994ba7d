'use strict';

/**
* escapes JSON Pointer using ~0 for ~ and ~1 for /
* @param s the string to escape
* @return the escaped string
*/
function jpescape(s) {
    return s.replace(/\~/g, '~0').replace(/\//g, '~1');
}

/**
* unescapes JSON Pointer using ~0 for ~ and ~1 for /
* @param s the string to unescape
* @return the unescaped string
*/
function jpunescape(s) {
    return s.replace(/\~1/g, '/').replace(/~0/g, '~');
}

// JSON Pointer specification: http://tools.ietf.org/html/rfc6901

/**
* from obj, return the property with a JSON Pointer prop, optionally setting it
* to newValue
* @param obj the object to point into
* @param prop the JSON Pointer or JSON Reference
* @param newValue optional value to set the property to
* @return the found property, or false
*/
function jptr(obj, prop, newValue) {
    if (typeof obj === 'undefined') return false;
    if (!prop || typeof prop !== 'string' || (prop === '#')) return (typeof newValue !== 'undefined' ? newValue : obj);

    if (prop.indexOf('#')>=0) {
        let parts = prop.split('#');
        let uri = parts[0];
        if (uri) return false; // we do internal resolution only
        prop = parts[1];
        prop = decodeURIComponent(prop.slice(1).split('+').join(' '));
    }
    if (prop.startsWith('/')) prop = prop.slice(1);

    let components = prop.split('/');
    for (let i=0;i<components.length;i++) {
        components[i] = jpunescape(components[i]);

        let setAndLast = (typeof newValue !== 'undefined') && (i == components.length-1);

        let index = parseInt(components[i],10);
        if (!Array.isArray(obj) || isNaN(index) || (index.toString() !== components[i])) {
            index = (Array.isArray(obj) && components[i] === '-') ? -2 : -1;
        }
        else {
            components[i] = (i > 0) ? components[i-1] : ''; // backtrack to indexed property name
        }

        if ((index != -1) || obj.hasOwnProperty(components[i])) {
            if (index >= 0) {
                if (setAndLast) {
                    obj[index] = newValue;
                }
                obj = obj[index];
            }
            else if (index === -2) {
                if (setAndLast) {
                    if (Array.isArray(obj)) {
                        obj.push(newValue);
                    }
                    return newValue;
                }
                else return undefined;
            }
            else {
                if (setAndLast) {
                    obj[components[i]] = newValue;
                }
                obj = obj[components[i]];
            }
        }
        else {
            if ((typeof newValue !== 'undefined') && (typeof obj === 'object') &&
                (!Array.isArray(obj))) {
                obj[components[i]] = (setAndLast ? newValue : ((components[i+1] === '0' || components[i+1] === '-') ? [] : {}));
                obj = obj[components[i]];
            }
            else return false;
        }
    }
    return obj;
}

// simple object accessor using dotted notation and [] for array indices
function fetchFromObject(obj, prop, newValue) {
    //property not found
    if (typeof obj === 'undefined') return false;
	if (!prop) {
		if (typeof newValue != 'undefined') {
			obj = newValue;
		}
		return obj;
	}

	var props = prop.split('.');
	var arr = props[0].split(/[\[\]]+/);
	var index = -1;
	if (arr.length>1) {
		index = parseInt(arr[1],10);
	}

    //property split found; recursive call
    if (props.length>1) {
		var pos = prop.indexOf('.');
        //get object at property (before split), pass on remainder
		if (index>=0) {
			return fetchFromObject(obj[arr[0]][index], prop.substr(pos+1), newValue); //was props
		}
		else {
			return fetchFromObject(obj[arr[0]], prop.substr(pos+1), newValue);
		}
	}
	//no split; get property[index] or property
	var source = obj;
	if (arr[0]) source = obj[prop];
	if (index>=0) {
		if (typeof newValue != 'undefined') source[index] = newValue;
		return source[index];
	}
    else {
		if (typeof newValue != 'undefined') obj[prop] = newValue;
		return obj[prop];
	}
}

function traverse(obj,prefix,depth,parent) {

var result = [];

	for (var key in obj) {
		// skip loop if the property is from prototype
		if (!obj.hasOwnProperty(key)) continue;

		var display = key;
		var sep = '.';
		if (Array.isArray(obj)) {
			display = '['+key+']';
			sep = '';
		}

		var item = {};
		item.prefix = prefix;
		item.key = key;
		item.display = display;
		item.value = obj[key];
		item.depth = depth;
		item.parent = parent;
		result.push(item);
		if (typeof obj[key] === 'object') {
			result = result.concat(traverse(obj[key],prefix+sep+display,depth+1,obj));
		}
	}
	return result;
}

function path(item,bracketed) {
	if (bracketed) {
		var result = '';
		var parents = item.prefix.split('.');
		for (var p=0;p<parents.length;p++) {
			result += "['" + parents[p] + "']";
		}
		if (item.display.charAt(0) == '[') {
			result += item.display;
		}
		else {
			result += '[' + item.display + ']';
		}
		return result;
	}
	else {
		var sep = '.';
		if ((typeof(item.value) === 'object') && (Array.isArray(item.parent)) && (item.prefix != '$')) {
			sep = '';
		}
		if (item.display.charAt(0) == '[') {
			sep = '';
		}
		return item.prefix+sep+item.display;
	}
}

function selectRegex(tree,expr,bracketed) {
	// not currently working, we are going to need some serious escaping of the regex
	if (!expr) {
		expr = /[*]/;
	}
	var result = [];
	for (var i=0;i<tree.length;i++) {
		var p = path(tree[i],bracketed);
		if (p.match(expr)) {
			result.push(tree[i]);
		}
	}
	return result;
}

function select(tree,target,bracketed) {
	var result = [];
	var returnParent = false;
	var checkEnd = false;

	// ^
	if (target.endsWith('^')) { // unoffical JSONPath extension
		target = target.substring(0,target.length-1);
		returnParent = true;
	}
	// .*
	if (target.endsWith('.*') && (target != '$..*')) {
		target = target.substring(0,target.length-2);
	}
	// [*]
	target = target.split('[*]').join('[]');
	// ..
	if ((target.indexOf('..') > 0) && (target != '$..*')) {
		var x = target.split('..');
		target = x[x.length-1];
		target = target.split('$').join('');
		checkEnd = true;
	}

	for (var i=0;i<tree.length;i++) {
		var p = path(tree[i],bracketed);
		if ((target == '*') || (target == '$..*') || (p == target) || ((p.endsWith(target) && checkEnd))) {
			if (returnParent) {
				result.push(tree[i].parent);
			}
			else {
				result.push(tree[i]);
			}
		}
	}
	return result;
}

module.exports = {
	build : function(obj) {
		return traverse(obj,'$',0,{});
	},
	select : select,
	selectRegex : selectRegex,
	path : path,
	fetchFromObject : fetchFromObject,
	jptr : jptr,
	jpescape : jpescape
};
