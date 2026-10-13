"use strict";

const test = require("node:test");
const assert = require("node:assert/strict");
const { csvReport, tsvReport } = require("../report.js");

const rows = [
  { id: 1, customer: "Ada", total: 12.5 },
  { id: 2, customer: 'Grace "Amazing", R.', total: null },
];

test("csv", () => {
  assert.equal(csvReport(rows), 'id,customer,total\n1,Ada,12.5\n2,"Grace ""Amazing"", R.",\n');
});

test("tsv", () => {
  assert.equal(tsvReport(rows), 'id\tcustomer\ttotal\n1\tAda\t12.5\n2\t"Grace ""Amazing"", R."\t\n');
});
