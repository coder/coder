"use strict";

require("core-js/modules/es.array.iterator");

exports.__esModule = true;
exports.makeTree = makeTree;
exports.filterRedundantErrors = filterRedundantErrors;
exports.createErrorInstances = createErrorInstances;
exports.default = void 0;

var _utils = require("./utils");

var _validationErrors = require("./validation-errors");

const JSON_POINTERS_REGEX = /\/[\w_-]+(\/\d+)?/g; // Make a tree of errors from ajv errors array

function makeTree(ajvErrors = []) {
  const root = {
    children: {}
  };
  ajvErrors.forEach(ajvError => {
    const {
      dataPath
    } = ajvError; // `dataPath === ''` is root

    const paths = dataPath === '' ? [''] : dataPath.match(JSON_POINTERS_REGEX);
    paths && paths.reduce((obj, path, i) => {
      obj.children[path] = obj.children[path] || {
        children: {},
        errors: []
      };

      if (i === paths.length - 1) {
        obj.children[path].errors.push(ajvError);
      }

      return obj.children[path];
    }, root);
  });
  return root;
}

function filterRedundantErrors(root, parent, key) {
  /**
   * If there is a `required` error then we can just skip everythig else.
   * And, also `required` should have more priority than `anyOf`. @see #8
   */
  (0, _utils.getErrors)(root).forEach(error => {
    if ((0, _utils.isRequiredError)(error)) {
      root.errors = [error];
      root.children = {};
    }
  });
  /**
   * If there is an `anyOf` error that means we have more meaningful errors
   * inside children. So we will just remove all errors from this level.
   *
   * If there are no children, then we don't delete the errors since we should
   * have at least one error to report.
   */

  if ((0, _utils.getErrors)(root).some(_utils.isAnyOfError)) {
    if (Object.keys(root.children).length > 0) {
      delete root.errors;
    }
  }
  /**
   * If all errors are `enum` and siblings have any error then we can safely
   * ignore the node.
   *
   * **CAUTION**
   * Need explicit `root.errors` check because `[].every(fn) === true`
   * https://en.wikipedia.org/wiki/Vacuous_truth#Vacuous_truths_in_mathematics
   */


  if (root.errors && root.errors.length && (0, _utils.getErrors)(root).every(_utils.isEnumError)) {
    if ((0, _utils.getSiblings)(parent)(root) // Remove any reference which becomes `undefined` later
    .filter(_utils.notUndefined).some(_utils.getErrors)) {
      delete parent.children[key];
    }
  }

  Object.entries(root.children).forEach(([key, child]) => filterRedundantErrors(child, root, key));
}

function createErrorInstances(root, options) {
  const errors = (0, _utils.getErrors)(root);

  if (errors.length && errors.every(_utils.isEnumError)) {
    const uniqueValues = new Set((0, _utils.concatAll)([])(errors.map(e => e.params.allowedValues)));
    const allowedValues = [...uniqueValues];
    const error = errors[0];
    return [new _validationErrors.EnumValidationError(Object.assign({}, error, {
      params: {
        allowedValues
      }
    }), options)];
  } else {
    return (0, _utils.concatAll)(errors.reduce((ret, error) => {
      switch (error.keyword) {
        case 'additionalProperties':
          return ret.concat(new _validationErrors.AdditionalPropValidationError(error, options));

        case 'required':
          return ret.concat(new _validationErrors.RequiredValidationError(error, options));

        default:
          return ret.concat(new _validationErrors.DefaultValidationError(error, options));
      }
    }, []))((0, _utils.getChildren)(root).map(child => createErrorInstances(child, options)));
  }
}

var _default = (ajvErrors, options) => {
  const tree = makeTree(ajvErrors || []);
  filterRedundantErrors(tree);
  return createErrorInstances(tree, options);
};

exports.default = _default;