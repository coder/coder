"use strict";

const COLUMNS = ["id", "customer", "total"];

function csvReport(rows) {
  const lines = [COLUMNS.join(",")];
  for (const row of rows) {
    const cells = COLUMNS.map((col) => {
      const value = row[col] === undefined || row[col] === null ? "" : String(row[col]);
      return value.includes(",") || value.includes('"') ? '"' + value.replace(/"/g, '""') + '"' : value;
    });
    lines.push(cells.join(","));
  }
  return lines.join("\n") + "\n";
}

function tsvReport(rows) {
  const lines = [COLUMNS.join("\t")];
  for (const row of rows) {
    const cells = COLUMNS.map((col) => {
      const value = row[col] === undefined || row[col] === null ? "" : String(row[col]);
      return value.includes("\t") || value.includes('"') ? '"' + value.replace(/"/g, '""') + '"' : value;
    });
    lines.push(cells.join("\t"));
  }
  return lines.join("\n") + "\n";
}

module.exports = { csvReport, tsvReport };
