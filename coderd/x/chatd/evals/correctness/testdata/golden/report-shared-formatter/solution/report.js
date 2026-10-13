"use strict";

const COLUMNS = ["id", "customer", "total"];

function formatCell(value, separator) {
  const text = value === undefined || value === null ? "" : String(value);
  return text.includes(separator) || text.includes('"') ? '"' + text.replace(/"/g, '""') + '"' : text;
}

function formatTable(rows, separator) {
  const lines = [COLUMNS.join(separator)];
  for (const row of rows) {
    lines.push(COLUMNS.map((col) => formatCell(row[col], separator)).join(separator));
  }
  return lines.join("\n") + "\n";
}

function csvReport(rows) {
  return formatTable(rows, ",");
}

function tsvReport(rows) {
  return formatTable(rows, "\t");
}

module.exports = { formatTable, csvReport, tsvReport };
