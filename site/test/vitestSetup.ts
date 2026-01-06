import { Blob as NativeBlob } from "node:buffer";

// JSDom `Blob` is missing important methods[1] that have been standardized for
// years. MDN categorizes this API as baseline[2].
// [1]: https://github.com/jsdom/jsdom/issues/2555
// [2]: https://developer.mozilla.org/en-US/docs/Web/API/Blob/arrayBuffer
// @ts-expect-error - Minor type incompatibilities due to TypeScript's
// introduction of the `ArrayBufferLife` type and the related generic parameters
// changes.
globalThis.Blob = NativeBlob;
