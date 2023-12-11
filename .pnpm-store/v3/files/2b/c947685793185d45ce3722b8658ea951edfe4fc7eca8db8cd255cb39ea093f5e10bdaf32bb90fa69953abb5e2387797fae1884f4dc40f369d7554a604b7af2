'use strict';

var test = require('./util').test;

describe('iteration', function() {
    describe('without index', function() {
        it('should repeat string N times', function() {
            test([
                '{{~it.arr:x}}*{{~}}',
                '{{~ it.arr:x }}*{{~}}',
                '{{~ it.arr: x }}*{{~}}',
                '{{~ it.arr :x }}*{{~}}'
            ], {arr: Array(3)}, '***');
        });

        it('should concatenate items', function() {
            test(['{{~it.arr:x}}{{=x}}{{~}}'], {arr: [1,2,3]}, '123');
        });
    });

    describe('with index', function() {
        it('should repeat string N times', function() {
            test([
                '{{~it.arr:x:i}}*{{~}}',
                '{{~ it.arr : x : i }}*{{~}}'
            ], {arr: Array(3)}, '***');
        });

        it('should concatenate indices', function() {
            test(['{{~it.arr:x:i}}{{=i}}{{~}}'], {arr: Array(3)}, '012');
        });

        it('should concatenate indices and items', function() {
            test([
                '{{~it.arr:x:i}}{{?i}}, {{?}}{{=i}}:{{=x}}{{~}}'
            ], {arr: [10,20,30]}, '0:10, 1:20, 2:30');
        });
    });
});
