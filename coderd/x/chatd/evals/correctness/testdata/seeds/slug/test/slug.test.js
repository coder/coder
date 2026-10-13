"use strict";

const test = require("node:test");
const assert = require("node:assert/strict");
const { slugify } = require("../slug.js");

test("basic titles", () => {
  assert.equal(slugify("Hello, World!"), "hello-world");
  assert.equal(slugify("  Crème brûlée  "), "creme-brulee");
});
