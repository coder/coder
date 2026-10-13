"use strict";

const test = require("node:test");
const assert = require("node:assert/strict");
const { slugify } = require("../slug.js");

test("maxLength cuts at a word boundary", () => {
  assert.equal(slugify("The quick brown fox jumps", { maxLength: 15 }), "the-quick-brown");
  assert.equal(slugify("The quick brown fox jumps", { maxLength: 14 }), "the-quick");
  assert.equal(slugify("Supercalifragilistic", { maxLength: 5 }), "super");
});

test("maxLength is optional", () => {
  assert.equal(slugify("The quick brown fox"), "the-quick-brown-fox");
  assert.equal(slugify("Short", { maxLength: 50 }), "short");
});
