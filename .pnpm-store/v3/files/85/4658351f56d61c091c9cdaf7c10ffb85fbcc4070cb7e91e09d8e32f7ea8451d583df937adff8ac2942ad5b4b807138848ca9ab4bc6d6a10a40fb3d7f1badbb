'use strict';

var assert = require('assert');
var doT = require('..');
var fs = require('fs');


describe('doT.process', function() {
  beforeEach(function() {
    removeCompiledTemplateFiles();
  });

  afterEach(function() {
    removeCompiledTemplateFiles();
  });

  function removeCompiledTemplateFiles() {
    try { fs.unlinkSync('./test/templates/test.js'); } catch(e) {}
  }

  it('should compile all templates in folder', function() {
    const templates = doT.process({path: './test/templates'});
    var str = templates.test({data: 2});
    assert.equal(str, '21');

    var js = fs.statSync('./test/templates/test.js');
    assert.ok(js.isFile());

    // code below passes if the test is run without coverage using `npm run test-spec`
    // because source code of doT.encodeHTMLSource is used inside compiled templates

    // var fn = require('./templates/test.js');
    // var str = fn({data: 2});
    // assert.equal(str, '21');
  });

  
	it('should ignore varname with polluted object prototype', function() {
    var currentLog = console.log;
    console.log = log;
    var logged;
    
    Object.prototype.templateSettings = {varname: 'it=(console.log("executed"),{})'};

    try {
      const templates = doT.process({path: './test/templates'});
      assert.notEqual(logged, 'executed');
      // injected code can only be executed if undefined is passed to template function
      templates.test();
      assert.notEqual(logged, 'executed');
    } finally {
      console.log = currentLog;
    }
    
    function log(str) {
      logged = str;
    }
  });
});
