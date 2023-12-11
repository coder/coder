'use strict';

var doT = require('..');
var assert = require('assert');

describe('defines', function() {
    describe('without parameters', function() {
        it('should render define', function(){
            testDef('{{##def.tmp:<div>{{!it.foo}}</div>#}}{{#def.tmp}}');
        });

        it('should render define if it is passed to doT.compile', function() {
            testDef('{{#def.tmp}}', {tmp: '<div>{{!it.foo}}</div>'});
        });
    });

    describe('with parameters', function() {
        it('should render define', function(){
            testDef('{{##def.tmp:foo:<div>{{!foo}}</div>#}}{{ var bar = it.foo; }}{{# def.tmp:bar }}');
        });
    });

    function testDef(tmpl, defines) {
        var fn = doT.compile(tmpl, defines);
        assert.equal(fn({foo:'http'}), '<div>http</div>');
        assert.equal(fn({foo:'http://abc.com'}), '<div>http:&#47;&#47;abc.com</div>');
        assert.equal(fn({}), '<div></div>');
    }
});