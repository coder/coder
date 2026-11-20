import assert from "node:assert/strict";
import { test } from "node:test";
import { toolchainProblems } from "./check-toolchain.mjs";

const manifest = { engines: { node: ">=22" }, packageManager: "pnpm@10.33.2" };

test("the minimum node and the pinned pnpm pass", () => {
  assert.deepEqual(toolchainProblems(manifest, { node: "v22.23.4", pnpm: "10.33.2\n" }), []);
  assert.deepEqual(toolchainProblems(manifest, { node: "v24.22.3", pnpm: "10.33.2" }), []);
});

test("an older node or another pnpm fails with both named", () => {
  assert.deepEqual(toolchainProblems(manifest, { node: "v20.19.0", pnpm: "12.16.1" }), [
    "node 22 or later is required, found v20.19.0",
    "pnpm 10.33.2 is required, found 12.16.1",
  ]);
});

test("a manifest the check cannot read fails", () => {
  assert.equal(toolchainProblems({}, { node: "v24.0.0", pnpm: "10.33.2" }).length, 2);
  assert.equal(
    toolchainProblems(
      { engines: { node: "22.x" }, packageManager: "pnpm@10" },
      { node: "v24.0.0", pnpm: "10.33.2" },
    ).length,
    2,
  );
});
