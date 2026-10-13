"use strict";

const test = require("node:test");
const assert = require("node:assert/strict");
const report = require("../report.js");

test("formatTable is exported and shared", () => {
  assert.equal(typeof report.formatTable, "function");
  const rows = [{ id: 7, customer: "a;b", total: 1 }];
  assert.equal(report.formatTable(rows, ";"), 'id;customer;total\n7;"a;b";1\n');
  assert.equal(report.csvReport(rows), report.formatTable(rows, ","));
  assert.equal(report.tsvReport(rows), report.formatTable(rows, "\t"));
});

test("output unchanged", () => {
  const rows = [{ id: 3, customer: "tab\there", total: 0 }, {}];
  assert.equal(report.tsvReport(rows), 'id\tcustomer\ttotal\n3\t"tab\there"\t0\n\t\t\n');
  assert.equal(report.csvReport(rows), "id,customer,total\n3,tab\there,0\n,,\n");
});
