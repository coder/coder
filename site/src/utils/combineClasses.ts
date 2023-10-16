export const appendCSSString = (
  cssString: string,
  cssClass: string,
): string => {
  if (cssString === "") {
    return cssClass;
  }
  return `${cssString}  ${cssClass}`;
};

/**
 * @param classes May be an object or an array. When using an object, each key
 * is a css class name and its associated value indicates whether it should be
 * applied. When using an array, only truthy values are applied.
 * @returns A string with each class that should be included. Classes are
 * separated by two spaces. If no classes are included, returns `undefined`.
 * @example
 * combineClasses()                               // -> undefined
 * combineClasses({ text: true })                 // -> "text"
 * combineClasses({ text: true, success: true })  // -> "text  success"
 * combineClasses({ text: true, success: false }) // -> "text"
 * combineClasses([ "text", false, "" ])          // -> "text"
 */
export const combineClasses = (
  classes:
    | Array<string | false | undefined>
    | Record<string, boolean | undefined> = {},
): string | undefined => {
  let result = "";

  if (Array.isArray(classes)) {
    for (const cssClass of classes) {
      if (cssClass) {
        result = appendCSSString(result, cssClass);
      }
    }
  } else {
    for (const cssClass in classes) {
      const useClass = classes[cssClass];
      if (useClass) {
        result = appendCSSString(result, cssClass);
      }
    }
  }

  return result.length > 0 ? result : undefined;
};
