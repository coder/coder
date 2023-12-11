'use strict';

var test = require('./util').test;
var assert = require("assert")
var doT = require("..");


describe('doT', function(){
	var basictemplate = "<div>{{!it.foo}}</div>";
	var basiccompiled = doT.template(basictemplate);

	describe('.name', function (){
		it('should have a name', function(){
			assert.strictEqual(doT.name, 'doT');
		});
	});
	
	describe('#template()', function(){
		it('should return a function', function(){
			assert.equal(typeof basiccompiled, "function");
		});
	});

	describe('#()', function(){
		it('should render the template', function(){
			assert.equal(basiccompiled({foo:"http"}), "<div>http</div>");
			assert.equal(basiccompiled({foo:"http://abc.com"}), "<div>http:&#47;&#47;abc.com</div>");
			assert.equal(basiccompiled({}), "<div></div>");
		});
	});

	describe('encoding with doNotSkipEncoded=false', function() {
		it('should not replace &', function() {
			global._encodeHTML = undefined;
			doT.templateSettings.doNotSkipEncoded = false;
			var fn = doT.template('<div>{{!it.foo}}</div>');
			assert.equal(fn({foo:"&amp;"}), "<div>&amp;</div>");
		});
	});

	describe('interpolate 2 numbers', function() {
		it('should print numbers next to each other', function() {
			test([
				'{{=it.one}}{{=it.two}}',
				'{{= it.one}}{{= it.two}}',
				'{{= it.one }}{{= it.two }}'
			], {one:1, two: 2}, '12');
		});
	});

	describe('evaluate JavaScript', function() {
		it('should print numbers next to each other', function() {
			test([
				'{{ it.one = 1; it.two = 2; }}{{= it.one }}{{= it.two }}',
			], {}, '12');
		});
	});

	describe('encoding with doNotSkipEncoded=true', function() {
		it('should replace &', function() {
			global._encodeHTML = undefined;
			doT.templateSettings.doNotSkipEncoded = true;
			assert.equal(doT.template('<div>{{!it.foo}}</div>')({foo:"&amp;"}), "<div>&#38;amp;</div>");
			assert.equal(doT.template('{{!it.a}}')({a:"& < > / ' \""}), "&#38; &#60; &#62; &#47; &#39; &#34;");
			assert.equal(doT.template('{{!"& < > / \' \\""}}')(), "&#38; &#60; &#62; &#47; &#39; &#34;");
		});
	});

	describe('invalid JS in templates', function() {
		it('should throw exception', function() {
			assert.throws(function() {
				var fn = doT.template('<div>{{= foo + }}</div>');
			});
		});
	});
});
