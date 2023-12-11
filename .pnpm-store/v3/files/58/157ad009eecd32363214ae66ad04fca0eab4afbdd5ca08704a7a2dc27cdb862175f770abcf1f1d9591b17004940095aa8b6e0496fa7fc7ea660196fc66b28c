"use strict";

exports.__esModule = true;
exports.concatAll = exports.getSiblings = exports.getChildren = exports.getErrors = exports.isEnumError = exports.isAnyOfError = exports.isRequiredError = exports.notUndefined = void 0;

// @flow

/*::
import type { Error, Node } from './types';
*/
// Basic
const eq = x => y => x === y;

const not = fn => x => !fn(x); // https://github.com/facebook/flow/issues/2221


const getValues =
/*::<Obj: Object>*/
(o
/*: Obj*/
) =>
/*: $ReadOnlyArray<$Values<Obj>>*/
Object.values(o);

const notUndefined = (x
/*: mixed*/
) => x !== undefined; // Error


exports.notUndefined = notUndefined;

const isXError = x => (error
/*: Error */
) => error.keyword === x;

const isRequiredError = isXError('required');
exports.isRequiredError = isRequiredError;
const isAnyOfError = isXError('anyOf');
exports.isAnyOfError = isAnyOfError;
const isEnumError = isXError('enum');
exports.isEnumError = isEnumError;

const getErrors = (node
/*: Node*/
) => node && node.errors || []; // Node


exports.getErrors = getErrors;

const getChildren = (node
/*: Node*/
) =>
/*: $ReadOnlyArray<Node>*/
node && getValues(node.children) || [];

exports.getChildren = getChildren;

const getSiblings = (parent
/*: Node*/
) => (node
/*: Node*/
) =>
/*: $ReadOnlyArray<Node>*/
getChildren(parent).filter(not(eq(node)));

exports.getSiblings = getSiblings;

const concatAll =
/*::<T>*/
(xs
/*: $ReadOnlyArray<T>*/
) => (ys
/*: $ReadOnlyArray<T>*/
) =>
/*: $ReadOnlyArray<T>*/
ys.reduce((zs, z) => zs.concat(z), xs);

exports.concatAll = concatAll;