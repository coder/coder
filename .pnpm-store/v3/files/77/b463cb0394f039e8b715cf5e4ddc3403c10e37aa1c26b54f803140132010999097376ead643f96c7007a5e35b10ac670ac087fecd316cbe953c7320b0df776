"use strict";

require("core-js/modules/es.array.concat");

require("core-js/modules/es.array.filter");

require("core-js/modules/es.object.values");

exports.__esModule = true;
exports.concatAll = exports.getSiblings = exports.getChildren = exports.getErrors = exports.isEnumError = exports.isAnyOfError = exports.isRequiredError = exports.notUndefined = void 0;

// @flow

/*::
import type { Error, Node } from './types';
*/
// Basic
var eq = function eq(x) {
  return function (y) {
    return x === y;
  };
};

var not = function not(fn) {
  return function (x) {
    return !fn(x);
  };
}; // https://github.com/facebook/flow/issues/2221


var getValues =
/*::<Obj: Object>*/
function getValues(o
/*: Obj*/
) {
  return (
    /*: $ReadOnlyArray<$Values<Obj>>*/
    Object.values(o)
  );
};

var notUndefined = function notUndefined(x
/*: mixed*/
) {
  return x !== undefined;
}; // Error


exports.notUndefined = notUndefined;

var isXError = function isXError(x) {
  return function (error
  /*: Error */
  ) {
    return error.keyword === x;
  };
};

var isRequiredError = isXError('required');
exports.isRequiredError = isRequiredError;
var isAnyOfError = isXError('anyOf');
exports.isAnyOfError = isAnyOfError;
var isEnumError = isXError('enum');
exports.isEnumError = isEnumError;

var getErrors = function getErrors(node
/*: Node*/
) {
  return node && node.errors || [];
}; // Node


exports.getErrors = getErrors;

var getChildren = function getChildren(node
/*: Node*/
) {
  return (
    /*: $ReadOnlyArray<Node>*/
    node && getValues(node.children) || []
  );
};

exports.getChildren = getChildren;

var getSiblings = function getSiblings(parent
/*: Node*/
) {
  return function (node
  /*: Node*/
  ) {
    return (
      /*: $ReadOnlyArray<Node>*/
      getChildren(parent).filter(not(eq(node)))
    );
  };
};

exports.getSiblings = getSiblings;

var concatAll =
/*::<T>*/
function concatAll(xs
/*: $ReadOnlyArray<T>*/
) {
  return function (ys
  /*: $ReadOnlyArray<T>*/
  ) {
    return (
      /*: $ReadOnlyArray<T>*/
      ys.reduce(function (zs, z) {
        return zs.concat(z);
      }, xs)
    );
  };
};

exports.concatAll = concatAll;