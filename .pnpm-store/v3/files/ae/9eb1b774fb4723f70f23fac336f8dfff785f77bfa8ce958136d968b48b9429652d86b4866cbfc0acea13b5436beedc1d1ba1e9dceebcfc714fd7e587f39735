"use strict";

require("core-js/modules/es.array.concat");

require("core-js/modules/es.array.filter");

require("core-js/modules/es.array.iterator");

require("core-js/modules/es.array.map");

require("core-js/modules/es.object.assign");

require("core-js/modules/es.object.entries");

require("core-js/modules/es.object.to-string");

require("core-js/modules/es.set");

require("core-js/modules/es.string.match");

exports.__esModule = true;
exports.makeTree = makeTree;
exports.filterRedundantErrors = filterRedundantErrors;
exports.createErrorInstances = createErrorInstances;
exports.default = void 0;

var _utils = require("./utils");

var _validationErrors = require("./validation-errors");

var JSON_POINTERS_REGEX = /\/[\w_-]+(\/\d+)?/g; // Make a tree of errors from ajv errors array

function makeTree(ajvErrors) {
  if (ajvErrors === void 0) {
    ajvErrors = [];
  }

  var root = {
    children: {}
  };
  ajvErrors.forEach(function (ajvError) {
    var dataPath = ajvError.dataPath; // `dataPath === ''` is root

    var paths = dataPath === '' ? [''] : dataPath.match(JSON_POINTERS_REGEX);
    paths && paths.reduce(function (obj, path, i) {
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
  (0, _utils.getErrors)(root).forEach(function (error) {
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

  Object.entries(root.children).forEach(function (_ref) {
    var key = _ref[0],
        child = _ref[1];
    return filterRedundantErrors(child, root, key);
  });
}

function createErrorInstances(root, options) {
  var errors = (0, _utils.getErrors)(root);

  if (errors.length && errors.every(_utils.isEnumError)) {
    var uniqueValues = new Set((0, _utils.concatAll)([])(errors.map(function (e) {
      return e.params.allowedValues;
    })));
    var allowedValues = [].concat(uniqueValues);
    var error = errors[0];
    return [new _validationErrors.EnumValidationError(Object.assign({}, error, {
      params: {
        allowedValues
      }
    }), options)];
  } else {
    return (0, _utils.concatAll)(errors.reduce(function (ret, error) {
      switch (error.keyword) {
        case 'additionalProperties':
          return ret.concat(new _validationErrors.AdditionalPropValidationError(error, options));

        case 'required':
          return ret.concat(new _validationErrors.RequiredValidationError(error, options));

        default:
          return ret.concat(new _validationErrors.DefaultValidationError(error, options));
      }
    }, []))((0, _utils.getChildren)(root).map(function (child) {
      return createErrorInstances(child, options);
    }));
  }
}

var _default = function _default(ajvErrors, options) {
  var tree = makeTree(ajvErrors || []);
  filterRedundantErrors(tree);
  return createErrorInstances(tree, options);
};

exports.default = _default;