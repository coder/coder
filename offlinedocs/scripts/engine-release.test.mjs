import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { test } from "node:test";
import {
  ENGINE_RELEASE_TAG,
  engineDependencies,
  engineDependencyProblems,
  engineReleaseBaseUrl,
  withEngineDependencies,
} from "./engine-release.mjs";

test("the tarballs are the release's assets on coder/docs-engine", () => {
  assert.equal(
    engineDependencies("v1.2.3")["@coder/docs-next"],
    "https://github.com/coder/docs-engine/releases/download/v1.2.3/coder-docs-next-1.2.3.tgz",
  );
  assert.equal(Object.keys(engineDependencies("v1.2.3")).length, 8);
});

test("only a vX.Y.Z tag is a release", () => {
  for (const tag of ["1.2.3", "v1.2", "v1.2.3-rc.0", "main", "v1.2.3/../x"]) {
    assert.throws(() => engineReleaseBaseUrl(tag), /not a release tag/, tag);
  }
});

test("package.json depends on exactly the pinned release", () => {
  const manifest = JSON.parse(readFileSync(new URL("../package.json", import.meta.url), "utf8"));
  assert.deepEqual(engineDependencyProblems(manifest, ENGINE_RELEASE_TAG), []);
});

test("a stale, missing, extra or misplaced engine dependency is reported", () => {
  const manifest = withEngineDependencies({ dependencies: { next: "16.4.5" } }, "v1.0.0");
  assert.deepEqual(engineDependencyProblems(manifest, "v1.0.0"), []);
  assert.equal(engineDependencyProblems(manifest, "v1.0.1").length, 8);

  const broken = structuredClone(manifest);
  delete broken.dependencies["@coder/docs-ui"];
  broken.dependencies["@coder/docs-extra"] = "1.0.0";
  broken.devDependencies = { "@coder/docs-config": "1.0.0" };
  assert.deepEqual(engineDependencyProblems(broken, "v1.0.0"), [
    "@coder/docs-ui should be https://github.com/coder/docs-engine/releases/download/v1.0.0/coder-docs-ui-1.0.0.tgz, is missing",
    "@coder/docs-extra is not a docs-engine release package",
    "devDependencies.@coder/docs-config belongs in dependencies",
  ]);
});

test("updating keeps the other dependencies and sorts them", () => {
  const updated = withEngineDependencies({ dependencies: { react: "19.2.7" } }, "v2.0.0");
  assert.deepEqual(Object.keys(updated.dependencies), [
    ...Object.keys(engineDependencies("v2.0.0")),
    "react",
  ]);
});
