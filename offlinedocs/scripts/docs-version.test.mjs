import assert from "node:assert/strict";
import { test } from "node:test";
import { docsVersion } from "./docs-version.mjs";

test("a release names its own version", () => {
  assert.equal(docsVersion("2.39.2"), "v2.39.2");
  assert.equal(docsVersion("v2.39.2"), "v2.39.2");
});

test("a release candidate names its minor line", () => {
  assert.equal(docsVersion("2.40.0-rc.0"), "v2.40");
  assert.equal(docsVersion("v2.40.0-rc.12"), "v2.40");
});

test("a development build is main", () => {
  assert.equal(docsVersion("2.39.2-devel+1a2b3c4d"), "main");
  assert.equal(docsVersion("0.0.0+sl-0123456789abcdef"), "main");
});

test("anything else fails", () => {
  for (const version of ["", "main", "2.39", "2.40.0-beta.1", "2.39.2-devel", "latest"]) {
    assert.throws(() => docsVersion(version), /cannot map coder version/, version);
  }
});
