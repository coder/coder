/* eslint-disable eslint-comments/disable-enable-pair -- make the line below works */
/* eslint-disable @typescript-eslint/ban-ts-comment -- it is a mjs module */
// @ts-nocheck
const editor = {
  defineTheme: () => {
    //
  },
  create: () => {
    return {
      dispose: () => {
        //
      },
    }
  },
}

const monaco = {
  editor,
}

module.exports = monaco
