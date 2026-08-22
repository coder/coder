import assert from "node:assert/strict";
import { test } from "node:test";

import { isHtml, type MdastNode } from "./mdast";

test("isHtml accepts an html node with a string value", () => {
	assert.equal(isHtml({ type: "html", value: "<div>" }), true);
	// An empty string is still a string, so an empty raw html node qualifies.
	assert.equal(isHtml({ type: "html", value: "" }), true);
});

test("isHtml rejects non-html nodes, valueless nodes, and undefined", () => {
	assert.equal(isHtml({ type: "html" }), false);
	assert.equal(isHtml({ type: "text", value: "x" }), false);
	assert.equal(isHtml({ type: "paragraph" } as MdastNode), false);
	assert.equal(isHtml(undefined), false);
});
