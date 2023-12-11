'use strict';

var util = require('util');
var debuglog = util.debuglog('jgexml');

var target; // for new properties
var attributePrefix = '@';
var laxURIs = false;
var defaultNameSpace = '';
var xsPrefix = 'xs:';

function reset(attrPrefix, laxURIprocessing, newXsPrefix) {
    target = null;
    attributePrefix = attrPrefix;
    laxURIs = laxURIprocessing;
    defaultNameSpace = '';
    xsPrefix = newXsPrefix || 'xs:';
}

function clone(obj) {
    return JSON.parse(JSON.stringify(obj));
}

function hoik(obj, target, key, newKey) {
    if (target && obj && (typeof obj[key] != 'undefined')) {
        if (!newKey) {
            newKey = key;
        }
        target[newKey] = clone(obj[key]);
        delete obj[key];
    }
}

function rename(obj, key, newName) {
    obj[newName] = obj[key];
    delete obj[key];
}

function isEmpty(obj) {
    if (typeof obj !== 'object') return false;
    for (var prop in obj) {
        if ((obj.hasOwnProperty(prop) && (typeof obj[prop] !== 'undefined'))) {
            return false;
        }
    }
    return true;
}

function toArray(item) {
    if (!(item instanceof Array)) {
        var newitem = [];
        if (item) {
            newitem.push(item);
        }
        return newitem;
    }
    else {
        return item;
    }
}

function mandate(target, inAnyOf, inAllOf, name) {
    if ((name != '#text') && (name != '#')) {
        var tempTarget = target;
        if (inAnyOf >= 0) {
            tempTarget = target.anyOf[inAnyOf];
        }
        if (inAllOf >= 0) {
            tempTarget = target.allOf[inAllOf];
        }
        if (!tempTarget.required) tempTarget.required = [];
        if (tempTarget.required.indexOf(name) < 0) {
            tempTarget.required.push(name);
        }
    }
}

function finaliseType(typeData) {
    if ((typeData.type == 'string') || (typeData.type == 'boolean') || (typeData.type == 'array') || (typeData.type == 'object')
        || (typeData.type == 'integer') || (typeData.type == 'number') || (typeData.type == 'null')) {
        //typeData.type = typeData.type;
    }
    else {
        if (typeData.type.startsWith('xml:')) { // id, lang, space, base, Father
            typeData.type = 'string';
        }
        else {
            var tempType = typeData.type;
            if (defaultNameSpace) {
                tempType = tempType.replace(defaultNameSpace + ':', '');
            }
            if (tempType.indexOf(':') >= 0) {
                var tempComp = tempType.split(':');
                typeData["$ref"] = tempComp[0] + '.json#/definitions/' + tempComp[1]; //'/'+typeData.type.replace(':','/');
            }
            else {
                typeData["$ref"] = '#/definitions/' + tempType;
            }
            delete typeData.type;
        }
    }
    return typeData;
}

function mapType(type) {

    var result = {};
    result.type = type;

    if (Array.isArray(type)) {
        result.type = 'object';
        result.oneOf = [];
        for (var t in type) {
            result.oneOf.push(finaliseType(mapType(type[t])));
        }
    }
    else if (type == xsPrefix + 'integer') {
        result.type = 'integer';
    }
    else if (type == xsPrefix + 'positiveInteger') {
        result.type = 'integer';
        result.minimum = 1;
    }
    else if (type == xsPrefix + 'nonPositiveInteger') {
        result.type = 'integer';
        result.maximum = 0;
    }
    else if (type == xsPrefix + 'negativeInteger') {
        result.type = 'integer';
        result.maximum = -1;
    }
    else if (type == xsPrefix + 'nonNegativeInteger') {
        result.type = 'integer';
        result.minimum = 0;
    }
    else if (type == xsPrefix + 'unsignedInt') {
        result.type = 'integer';
        result.minimum = 0;
        result.maximum = 4294967295;
    }
    else if (type == xsPrefix + 'unsignedShort') {
        result.type = 'integer';
        result.minimum = 0;
        result.maximum = 65535;
    }
    else if (type == xsPrefix + 'unsignedByte') {
        result.type = 'integer';
        result.minimum = 0;
        result.maximum = 255;
    }
    else if (type == xsPrefix + 'int') {
        result.type = 'integer';
        result.maximum = 2147483647;
        result.minimum = -2147483648;
    }
    else if (type == xsPrefix + 'short') {
        result.type = 'integer';
        result.maximum = 32767;
        result.minimum = -32768;
    }
    else if (type == xsPrefix + 'byte') {
        result.type = 'integer';
        result.maximum = 127;
        result.minimum = -128;
    }
    else if (type == xsPrefix + 'long') {
        result.type = 'integer';
    }
    else if (type == xsPrefix + 'unsignedLong') {
        result.type = 'integer';
        result.minimum = 0;
    }

    if (type == xsPrefix + 'string') result.type = 'string';
    if (type == xsPrefix + 'NMTOKEN') result.type = 'string';
    if (type == xsPrefix + 'NMTOKENS') result.type = 'string';
    if (type == xsPrefix + 'ENTITY') result.type = 'string';
    if (type == xsPrefix + 'ENTITIES') result.type = 'string';
    if (type == xsPrefix + 'ID') result.type = 'string';
    if (type == xsPrefix + 'IDREF') result.type = 'string';
    if (type == xsPrefix + 'IDREFS') result.type = 'string';
    if (type == xsPrefix + 'NOTATION') result.type = 'string';
    if (type == xsPrefix + 'token') result.type = 'string';
    if (type == xsPrefix + 'Name') result.type = 'string';
    if (type == xsPrefix + 'NCName') result.type = 'string';
    if (type == xsPrefix + 'QName') result.type = 'string';
    if (type == xsPrefix + 'normalizedString') result.type = 'string';
    if (type == xsPrefix + 'base64Binary') {
        result.type = 'string';
        result.format = 'byte';
    }
    if (type == xsPrefix + 'hexBinary') {
        result.type = 'string';
        result.format = '^[0-9,a-f,A-F]*';
    }

    if (type == xsPrefix + 'boolean') result.type = 'boolean';

    if (type == xsPrefix + 'date') {
        result.type = 'string';
        result.pattern = '^[0-9]{4}\-[0-9]{2}\-[0-9]{2}.*$'; //timezones
    }
    else if (type == xsPrefix + 'dateTime') {
        result.type = 'string';
        result.format = 'date-time';
    }
    else if (type == xsPrefix + 'time') {
        result.type = 'string';
        result.pattern = '^[0-9]{2}\:[0-9]{2}:[0-9]{2}.*$'; // timezones
    }
    else if (type == xsPrefix + 'duration') {
        result.type = 'string';
        result.pattern = '^(-)?P(?:([0-9,.]*)Y)?(?:([0-9,.]*)M)?(?:([0-9,.]*)W)?(?:([0-9,.]*)D)?(?:T(?:([0-9,.]*)H)?(?:([0-9,.]*)M)?(?:([0-9,.]*)S)?)?$';
    }
    else if (type == xsPrefix + 'gDay') {
        result.type = 'string';
        result.pattern = '[0-9]{2}';
    }
    else if (type == xsPrefix + 'gMonth') {
        result.type = 'string';
        result.pattern = '[0-9]{2}';
    }
    else if (type == xsPrefix + 'gMonthDay') {
        result.type = 'string';
        result.pattern = '[0-9]{2}\-[0-9]{2}';
    }
    else if (type == xsPrefix + 'gYear') {
        result.type = 'string';
        result.pattern = '[0-9]{4}';
    }
    else if (type == xsPrefix + 'gYearMonth') {
        result.type = 'string';
        result.pattern = '[0-9]{4}\-[0-9]{2}';
    }

    if (type == xsPrefix + 'language') {
        result.type = 'string';
        result.pattern = '[a-zA-Z]{1,8}(-[a-zA-Z0-9]{1,8})*';
    }

    if (type == xsPrefix + 'decimal') {
        result.type = 'number';
    }
    else if (type == xsPrefix + 'double') {
        result.type = 'number';
        result.format = 'double';
    }
    else if (type == xsPrefix + 'float') {
        result.type = 'number';
        result.format = 'float';
    }

    if (type == xsPrefix + 'anyURI') {
        result.type = 'string';
        if (!laxURIs) {
            result.format = 'uri'; //XSD allows relative URIs, it seems JSON schema uri format may not?
            // this regex breaks swagger validators
            //result.pattern = '^(([^:/?#]+):)?(//([^/?#]*))?([^?#]*)(\?([^#]*))?(#(.*))?';
        }
    }

    return result;
}

function initTarget(parent) {
    if (!target) target = parent;
    if (!target.properties) {
        target.properties = {};
        target.required = [];
        target.additionalProperties = false;
    }
    if (!target.allOf) target.allOf = [];
}

function doElement(src, parent, key) {
    var type = 'object';
    var name;

    var simpleType;
    var doc;
    var inAnyOf = -1; // used for attributeGroups - properties can get merged in here later, see mergeAnyOf
    var inAllOf = (target && target.allOf) ? target.allOf.length - 1 : -1; // used for extension based composition

    var element = src[key];
    if ((typeof element == 'undefined') || (null === element)) {
        return false;
    }

    if ((key == xsPrefix + "any") || (key == xsPrefix + "anyAttribute")) {
        if (target) target.additionalProperties = true; // target should always be defined at this point
    }

    if (element[xsPrefix + "annotation"]) {
        doc = element[xsPrefix + "annotation"][xsPrefix + "documentation"];
    }

    if (element["@name"]) {
        name = element["@name"];
    }
    if (element["@type"]) {
        type = element["@type"];
    }
    else if ((element["@name"]) && (element[xsPrefix + "simpleType"])) {
        type = element[xsPrefix + "simpleType"][xsPrefix + "restriction"]["@base"];
        simpleType = element[xsPrefix + "simpleType"][xsPrefix + "restriction"];
        if (element[xsPrefix + "simpleType"][xsPrefix + "annotation"]) {
            simpleType[xsPrefix + "annotation"] = element[xsPrefix + "simpleType"][xsPrefix + "annotation"];
        }
    }
    else if ((element["@name"]) && (element[xsPrefix + "restriction"])) {
        type = element[xsPrefix + "restriction"]["@base"];
        simpleType = element[xsPrefix + "restriction"];
        if (element[xsPrefix + "annotation"]) {
            simpleType[xsPrefix + "annotation"] = element[xsPrefix + "annotation"];
        }
    }
    else if ((element[xsPrefix + "extension"]) && (element[xsPrefix + "extension"]["@base"])) {
        type = element[xsPrefix + "extension"]["@base"];
        var tempType = finaliseType(mapType(type));
        if (!tempType["$ref"]) {
            name = "#text"; // see anonymous types
        }
        else {
            var oldP = clone(target);
            oldP.additionalProperties = true;
            for (var v in target) {
                delete target[v];
            }
            if (!target.allOf) target.allOf = [];
            var newt = {};
            target.allOf.push(newt);
            target.allOf.push(oldP);
            name = '#';
            inAllOf = 0; //target.allOf.length-1;
        }
    }
    else if (element[xsPrefix + "union"]) {
        var types = element[xsPrefix + "union"]["@memberTypes"].split(' ');
        type = [];
        for (var t in types) {
            type.push(types[t]);
        }
    }
    else if (element[xsPrefix + "list"]) {
        type = 'string';
    }
    else if (element["@ref"]) {
        name = element["@ref"];
        type = element["@ref"];
    }

    if (name && type) {
        var isAttribute = (element["@isAttr"] == true);

        initTarget(parent);
        var newTarget = target;

        var minOccurs = 1;
        var maxOccurs = 1;
        var enumList = [];
        if (element["@minOccurs"]) minOccurs = parseInt(element["@minOccurs"], 10);
        if (element["@maxOccurs"]) maxOccurs = element["@maxOccurs"];
        if (maxOccurs == 'unbounded') maxOccurs = Number.MAX_SAFE_INTEGER;
        if (isAttribute) {
            if ((!element["@use"]) || (element["@use"] != 'required')) minOccurs = 0;
            if (element["@fixed"]) enumList.push(element["@fixed"]);
        }
        if (element["@isChoice"]) minOccurs = 0;

        var typeData = mapType(type);
        if (isAttribute && (typeData.type == 'object')) {
            typeData.type = 'string'; // handle case where attribute has no defined type
        }

        if (doc) {
            typeData.description = doc;
        }

        if (enumList.length) {
            typeData.enum = enumList;
        }

        if (typeData.type == 'object') {
            typeData.properties = {};
            typeData.required = [];
            typeData.additionalProperties = false;
            newTarget = typeData;
        }

        // handle @ref / attributeGroups
        if ((key == xsPrefix + "attributeGroup") && (element["@ref"])) { // || (name == '$ref')) {
            if (!target.anyOf) target.anyOf = [];
            var newt = {};
            newt.properties = {};
            newt.required = clone(target.required);
            target.anyOf.push(newt);
            inAnyOf = target.anyOf.length - 1;
            target.required = [];
            delete src[key];
            minOccurs = 0;
        }

        if ((parent[xsPrefix + "annotation"]) && ((parent[xsPrefix + "annotation"][xsPrefix + "documentation"]))) {
            target.description = parent[xsPrefix + "annotation"][xsPrefix + "documentation"];
        }
        if ((element[xsPrefix + "annotation"]) && ((element[xsPrefix + "annotation"][xsPrefix + "documentation"]))) {
            target.description = (target.description ? target.decription + '\n' : '') + element[xsPrefix + "annotation"][xsPrefix + "documentation"];
        }

        var enumSource;

        if (element[xsPrefix + "simpleType"] && element[xsPrefix + "simpleType"][xsPrefix + "restriction"] && element[xsPrefix + "simpleType"][xsPrefix + "restriction"][xsPrefix + "enumeration"]) {
            var enumSource = element[xsPrefix + "simpleType"][xsPrefix + "restriction"][xsPrefix + "enumeration"];
        }
        else if (element[xsPrefix + "restriction"] && element[xsPrefix + "restriction"][xsPrefix + "enumeration"]) {
            var enumSource = element[xsPrefix + "restriction"][xsPrefix + "enumeration"];
        }

        if (enumSource) {
            typeData.description = '';
            typeData["enum"] = [];
            enumSource = toArray(enumSource); // handle 'const' case
            for (var i = 0; i < enumSource.length; i++) {
                typeData["enum"].push(enumSource[i]["@value"]);
                if ((enumSource[i][xsPrefix + "annotation"]) && (enumSource[i][xsPrefix + "annotation"][xsPrefix + "documentation"])) {
                    if (typeData.description) {
                        typeData.description += '';
                    }
                    typeData.description += enumSource[i]["@value"] + ': ' + enumSource[i][xsPrefix + "annotation"][xsPrefix + "documentation"];
                }
            }
            if (!typeData.description) delete typeData.description;
        }
        else {
            typeData = finaliseType(typeData);
        }

        if (maxOccurs > 1) {
            var newTD = {};
            newTD.type = 'array';
            if (minOccurs > 0) newTD.minItems = parseInt(minOccurs, 10);
            if (maxOccurs < Number.MAX_SAFE_INTEGER) newTD.maxItems = parseInt(maxOccurs, 10);
            newTD.items = typeData;
            typeData = newTD;
            // TODO add mode where if array minOccurs is 1, add oneOf allowing single object or array with object as item
        }
        if (minOccurs > 0) {
            mandate(target, inAnyOf, inAllOf, name);
        }

        if (simpleType) {
            if (simpleType[xsPrefix + "minLength"]) typeData.minLength = parseInt(simpleType[xsPrefix + "minLength"]["@value"], 10);
            if (simpleType[xsPrefix + "maxLength"]) typeData.maxLength = parseInt(simpleType[xsPrefix + "maxLength"]["@value"], 10);
            if (simpleType[xsPrefix + "pattern"]) typeData.pattern = simpleType[xsPrefix + "pattern"]["@value"];
            if ((simpleType[xsPrefix + "annotation"]) && (simpleType[xsPrefix + "annotation"][xsPrefix + "documentation"])) {
                typeData.description = simpleType[xsPrefix + "annotation"][xsPrefix + "documentation"];
            }
        }

        if (inAllOf >= 0) {
            if (typeData.$ref) target.allOf[inAllOf].$ref = typeData["$ref"]
            else delete target.allOf[inAllOf].$ref;
        }
        else if (inAnyOf >= 0) {
            if (typeData.$ref) target.anyOf[inAnyOf].$ref = typeData["$ref"]
            else delete target.anyOf[inAnyOf].$ref;
        }
        else {
            if (!target.type) target.type = 'object';
            target.properties[name] = typeData; // Object.assign 'corrupts' property ordering
        }

        target = newTarget;
    }
}

function moveAttributes(obj, parent, key) {
    if (key == xsPrefix + 'attribute') {

        obj[key] = toArray(obj[key]);

        var target;

        if (obj[xsPrefix + "sequence"] && obj[xsPrefix + "sequence"][xsPrefix + "element"]) {
            obj[xsPrefix + "sequence"][xsPrefix + "element"] = toArray(obj[xsPrefix + "sequence"][xsPrefix + "element"]);
            target = obj[xsPrefix + "sequence"][xsPrefix + "element"];
        }
        if (obj[xsPrefix + "choice"] && obj[xsPrefix + "choice"][xsPrefix + "element"]) {
            obj[xsPrefix + "choice"][xsPrefix + "element"] = toArray(obj[xsPrefix + "choice"][xsPrefix + "element"]);
            target = obj[xsPrefix + "choice"][xsPrefix + "element"];
        }

        for (var i = 0; i < obj[key].length; i++) {
            var attr = clone(obj[key][i]);
            if (attributePrefix) {
                attr["@name"] = attributePrefix + attr["@name"];
            }
            if (typeof attr == 'object') {
                attr["@isAttr"] = true;
            }
            if (target) target.push(attr)
            else obj[key][i] = attr;
        }
        if (target) delete obj[key];
    }
}

function processChoice(obj, parent, key) {
    if (key == xsPrefix + 'choice') {
        var e = obj[key][xsPrefix + "element"] = toArray(obj[key][xsPrefix + "element"]);
        for (var i = 0; i < e.length; i++) {
            if (!e[i]["@isAttr"]) {
                e[i]["@isChoice"] = true;
            }
        }
        if (obj[key][xsPrefix + "group"]) {
            var g = obj[key][xsPrefix + "group"] = toArray(obj[key][xsPrefix + "group"]);
            for (var i = 0; i < g.length; i++) {
                if (!g[i]["@isAttr"]) {
                    g[i]["@isChoice"] = true;
                }
            }
        }
    }
}

function renameObjects(obj, parent, key) {
    if (key == xsPrefix + 'complexType') {
        var name = obj["@name"];
        if (name) {
            rename(obj, key, name);
        }
        else debuglog('complexType with no name');
    }
}

function moveProperties(obj, parent, key) {
    if (key == xsPrefix + 'sequence') {
        if (obj[key].properties) {
            obj.properties = obj[key].properties;
            obj.required = obj[key].required;
            obj.additionalProperties = false;
            delete obj[key];
        }
    }
}

function clean(obj, parent, key) {
    if (key == '@name') delete obj[key];
    if (key == '@type') delete obj[key];
    if (key == xsPrefix + "attribute") delete obj[key];
    if (key == xsPrefix + "restriction") delete obj[key];
    if (obj.properties && (Object.keys(obj.properties).length == 1) && obj.properties["#text"] && obj.properties["#text"]["$ref"]) {
        obj.properties["$ref"] = obj.properties["#text"]["$ref"];
        delete obj.properties["#text"]; // anonymous types
    }
    if (obj.properties && obj.anyOf) { // mergeAnyOf
        var newI = {};
        if (obj.properties["$ref"]) {
            newI["$ref"] = obj.properties["$ref"];
        }
        else if (Object.keys(obj.properties).length > 0) {
            newI.properties = obj.properties;
            newI.required = obj.required;
        }
        if (Object.keys(newI).length > 0) {
            obj.anyOf.push(newI);
        }
        obj.properties = {}; // gets removed later
        obj.required = []; // ditto

        if (obj.anyOf.length == 1) {
            if (obj.anyOf[0]["$ref"]) {
                obj["$ref"] = clone(obj.anyOf[0]["$ref"]);
                delete obj.type;
                delete obj.additionalProperties;
            }
            // possible missing else here for properties !== {}
            obj.anyOf = []; // also gets removed later
        }
    }
}

function removeEmpties(obj, parent, key) {
    var count = 0;
    if (isEmpty(obj[key])) {
        delete obj[key];
        if (key == 'properties') {
            if ((!obj.oneOf) && (!obj.anyOf)) {
                if (obj.type == 'object') obj.type = 'string';
                delete obj.additionalProperties;
            }
        }
        count++;
    }
    else {
        if (Array.isArray(obj[key])) {
            var newArray = [];
            for (var i = 0; i < obj[key].length; i++) {
                if (typeof obj[key][i] !== 'undefined') {
                    newArray.push(obj[key][i]);
                }
                else {
                    count++;
                }
            }
            if (newArray.length == 0) {
                delete obj[key];
                count++;
            }
            else {
                obj[key] = newArray;
            }
        }
    }
    return count;
}

function recurse(obj, parent, callback, depthFirst) {

    var oTarget = target;

    if (typeof obj != 'string') {
        for (var key in obj) {
            target = oTarget;
            // skip loop if the property is from prototype
            if (!obj.hasOwnProperty(key)) continue;

            if (!depthFirst) callback(obj, parent, key);

            var array = Array.isArray(obj);

            if (typeof obj[key] === 'object') {
                if (array) {
                    for (var i in obj[key]) {
                        recurse(obj[key][i], obj[key], callback);
                    }
                }
                recurse(obj[key], obj, callback);
            }

            if (depthFirst) callback(obj, parent, key);
        }
    }

    return obj;
}

module.exports = {
    getJsonSchema: function getJsonSchema(src, title, outputAttrPrefix, laxURIs, newXsPrefix) { // TODO convert to options parameter
        reset(outputAttrPrefix, laxURIs, newXsPrefix);

        for (let p in src) {
            if (p.indexOf(':') >= 0) {
                let pp = p.split(':')[0];
                if (src[p]["@xmlns:" + pp] === 'http://www.w3.org/2001/XMLSchema') {
                    xsPrefix = pp + ':';
                }
            }
        }

        recurse(src, {}, function (src, parent, key) {
            moveAttributes(src, parent, key);
        });
        recurse(src, {}, function (src, parent, key) {
            processChoice(src, parent, key);
        });

        var obj = {};
        var id = '';

        if (src[xsPrefix + "schema"]) {
            id = src[xsPrefix + "schema"]["@targetNamespace"];
            if (!id) {
                id = src[xsPrefix + "schema"]["@xmlns"];
            }
        }
        else throw new Error('Could find schema with given prefix: ' + xsPrefix);

        for (var a in src[xsPrefix + "schema"]) {
            if (a.startsWith('@xmlns:')) {
                if (src[xsPrefix + "schema"][a] == id) {
                    defaultNameSpace = a.replace('@xmlns:', '');
                }
            }
        }

        //initial root object transformations
        obj.title = title;
        obj.$schema = 'http://json-schema.org/schema#'; //for latest, or 'http://json-schema.org/draft-04/schema#' for v4
        if (id) {
            obj.id = id;
        }
        if (src[xsPrefix + "schema"] && src[xsPrefix + "schema"][xsPrefix + "annotation"]) {
            obj.description = '';
            src[xsPrefix + "schema"][xsPrefix + "annotation"] = toArray(src[xsPrefix + "schema"][xsPrefix + "annotation"]);
            for (var a in src[xsPrefix + "schema"][xsPrefix + "annotation"]) {
                var annotation = src[xsPrefix + "schema"][xsPrefix + "annotation"][a];
                if ((annotation[xsPrefix + "documentation"]) && (annotation[xsPrefix + "documentation"]["#text"])) {
                    obj.description += (obj.description ? '\n' : '') + annotation[xsPrefix + "documentation"]["#text"];
                }
                else {
                    if (annotation[xsPrefix + "documentation"]) obj.description += (obj.description ? '\n' : '') + annotation[xsPrefix + "documentation"];
                }
            }
        }

        var rootElement = src[xsPrefix + "schema"][xsPrefix + "element"];
        if (Array.isArray(rootElement)) {
            rootElement = rootElement[0];
        }
        var rootElementName = rootElement["@name"];

        obj.type = 'object';
        obj.properties = clone(rootElement);

        obj.required = [];
        obj.required.push(rootElementName);
        obj.additionalProperties = false;

        recurse(obj, {}, function (obj, parent, key) {
            renameObjects(obj, parent, key);
        });

        // support for schemas with just a top-level name and type (no complexType/sequence etc)
        if (obj.properties["@type"]) {
            target = obj; // tell it where to put the properties
        }
        else {
            delete obj.properties["@name"]; // to prevent root-element being picked up twice
        }

        // main processing of the root element
        recurse(obj, {}, function (src, parent, key) { // was obj.properties
            doElement(src, parent, key);
        });

        recurse(obj, {}, function (obj, parent, key) {
            moveProperties(obj, parent, key);
        });

        // remove rootElement to leave ref'd definitions
        if (Array.isArray(src[xsPrefix + "schema"][xsPrefix + "element"])) {
            //src[xsPrefix+"schema"][xsPrefix+"element"] = src[xsPrefix+"schema"][xsPrefix+"element"].splice(0,1);
            delete src[xsPrefix + "schema"][xsPrefix + "element"][0];
        }
        else {
            delete src[xsPrefix + "schema"][xsPrefix + "element"];
        }

        obj.definitions = clone(src);
        obj.definitions.properties = {};
        target = obj.definitions;

        // main processing of the ref'd elements
        recurse(obj.definitions, {}, function (src, parent, key) {
            doElement(src, parent, key);
        });

        // correct for /definitions/properties
        obj.definitions = obj.definitions.properties;

        recurse(obj, {}, function (obj, parent, key) {
            clean(obj, parent, key);
        });

        delete (obj.definitions[xsPrefix + "schema"]);

        var count = 1;
        while (count > 0) { // loop until we haven't removed any empties
            count = 0;
            recurse(obj, {}, function (obj, parent, key) {
                count += removeEmpties(obj, parent, key);
            });
        }

        return obj;
    }
};
