"use strict";

require("core-js/modules/es.promise");

exports.__esModule = true;
exports.getSchemaAndData = getSchemaAndData;

var _fs = require("fs");

var _jestFixtures = require("jest-fixtures");

async function getSchemaAndData(name, dirPath) {
  const schemaPath = await (0, _jestFixtures.getFixturePath)(dirPath, name, 'schema.json');
  const schema = JSON.parse((0, _fs.readFileSync)(schemaPath, 'utf8'));
  const dataPath = await (0, _jestFixtures.getFixturePath)(dirPath, name, 'data.json');
  const data = JSON.parse((0, _fs.readFileSync)(dataPath, 'utf8'));
  return [schema, data];
}