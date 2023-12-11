// jslitmus.js
//
// Copyright (c) 2010, Robert Kieffer, http://broofa.com
// Available under MIT license (http://en.wikipedia.org/wiki/MIT_License)

(function() {
  var root = this;

  //
  // Platform detect
  //

  var platform = (function() {
    // Platform info object
    var p = {
      name: null,
      version: null,
      os: null,
      description: 'unknown platform',
      toString: function() {return this.description;}
    };

    if (root.navigator) {
      var ua = navigator.userAgent;

      // Detect OS
      var oses = 'Windows|iPhone OS|(?:Intel |PPC )?Mac OS X|Linux';
      p.os = new RegExp('((' + oses + ') +[^ \);]*)').test(ua) ? RegExp.$1.replace(/_/g, '.') : null;

      // Detect expected names
      p.name = /(Chrome|MSIE|Safari|Opera|Firefox|Minefield)/.test(ua) ? RegExp.$1 : null;

      // Detect version
      if (p.name == 'Opera') {
        p.version = opera.name;
      } else if (p.name) {
        var vre = new RegExp('(Version|' + p.name + ')[ \/]([^ ;]*)');
        p.version = vre.test(ua) ? RegExp.$2 : null;
      }
    } else if (root.process && process.platform) {
      // Support node.js (see http://nodejs.org)
      p.name = 'node';
      p.version = process.version;
      p.os = process.platform;
    }

    // Set the description
    var d = [];
    if (p.name) d.push(p.name);
    if (p.version) d.push(' ' + p.version);
    if (p.os) d.push(' on ' + p.os);
    if (d.length) p.description = d.join('');

    return p;
  })();

  //
  // Context-specific initialization
  //

  var sys = null, querystring = null;
  if (platform.name == 'node') {
    util = require('util');
    querystring = require('querystring');
  }

  //
  // Misc convenience methods
  //

  function log(msg) {
    if (typeof(console) != 'undefined') {
      console.log(msg);
    } else if (sys) {
      util.log(msg);
    }
  }

  // nil function
  function nilf(x) {
    return x;
  }

  // Copy properties
  function extend(dst, src) {
    for (var k in src) {
      dst[k] = src[k];
    }
    return dst;
  }

  // Array: apply f to each item in a
  function forEach(a, f) {
    for (var i = 0, il = (a && a.length); i < il; i++) {
      var o = a[i];
      f(o, i);
    }
  }

  // Array: return array of all results of f(item)
  function map(a, f) {
    var o, res = [];
    for (var i = 0, il = (a && a.length); i < il; i++) {
      var o = a[i];
      res.push(f(o, i));
    }
    return res;
  }

  // Array: filter out items for which f(item) is falsy
  function filter(a, f) {
    var o, res = [];
    for (var i = 0, il = (a && a.length); i < il; i++) {
      var o = a[i];
      if (f(o, i)) res.push(o);
    }
    return res;
  }

  // Array: IE doesn't have indexOf in some cases
  function indexOf(a, o) {
    if (a.indexOf) return a.indexOf(o);
    for (var i = 0, l = a.length; i < l; i++) if (a[i] === o) return i;
    return -1;
  }

  // Enhanced escape()
  function escape2(s) {
    s = s.replace(/,/g, '\\,');
    s = querystring ? querystring.escape(s) : escape(s);
    s = s.replace(/\+/g, '%2b');
    s = s.replace(/ /g, '+');
    return s;
  }

  // join(), for objects. Creates url query param-style strings by default
  function join(o, delimit1, delimit2) {
    var asQuery = !delimit1 && !delimit2;
    if (asQuery) {
      delimit1 = '&';
      delimit2 = '=';
    }

    var pairs = [];
    for (var key in o) {
      var value = o[key];
      if (asQuery) value = escape2(value);
      pairs.push(key + delimit2 + o[key]);
    }
    return pairs.join(delimit1);
  }

  // split(), for object strings. Parses url query param strings by default
  function split(s, delimit1, delimit2) {
    var asQuery = !delimit1 && !delimit2;
    if (asQuery) {
      s = s.replace(/.*[?#]/, '');
      delimit1 = '&';
      delimit2 = '=';
    }

    if (match) {
      var o = query.split(delimit1);
      for (var i = 0; i < o.length; i++) {
        var pair = o[i].split(new RegExp(delimit2 + '+'));
        var key = pair.shift();
        var value = (asQuery && pair.length > 1) ? pair.join(delimit2) : pair[0];
        o[key] = value;
      }
    }

    return o;
  }

  // Round x to d significant digits
  function sig(x, d) {
    var exp = Math.ceil(Math.log(Math.abs(x))/Math.log(10)),
        f = Math.pow(10, exp-d);
    return Math.round(x/f)*f;
  }

  // Convert x to a readable string version
  function humanize(x, sd) {
    var ax = Math.abs(x), res;
    sd = sd | 4;  // significant digits
    if (ax == Infinity) {
      res = ax > 0 ? 'Infinity' : '-Infinity';
    } else if (ax > 1e9) {
      res = sig(x/1e9, sd) + 'G';
    } else if (ax > 1e6) {
      res = sig(x/1e6, sd) + 'M';
    } else if (ax > 1e3) {
      res = sig(x/1e3, sd) + 'k';
    } else if (ax > .01) {
      res = sig(x, sd);
    } else if (ax > 1e-3) {
      res = sig(x/1e-3, sd) + 'm';
    } else if (ax > 1e-6) {
      res = sig(x/1e-6, sd) + '\u00b5'; // Greek mu
    } else if (ax > 1e-9) {
      res = sig(x/1e-9, sd) + 'n';
    } else {
      res = x ? sig(x, sd) : 0;
    }
    // Turn values like "1.1000000000005" -> "1.1"
    res = (res + '').replace(/0{5,}\d*/, '');

    return res;
  }

  // Node.js-inspired event emitter API, with some enhancements.
  function EventEmitter() {
    var ee = this;
    var listeners = {};
    extend(ee, {
      on: function(e, f) {
        if (!listeners[e]) listeners[e] = [];
        listeners[e].push(f);
      },
      removeListener: function(e, f) {
        listeners[e] = filter(listeners[e], function(l) {
          return l != f;
        });
      },
      removeAllListeners: function(e) {
        listeners[e] = [];
      },
      emit: function(e) {
        var args = Array.prototype.slice.call(arguments, 1);
        forEach([].concat(listeners[e], listeners['*']), function(l) {
          ee._emitting = e;
          if (l) l.apply(ee, args);
        });
        delete ee._emitting;
      }
    });
  }

  //
  // Test class
  //

  /**
   * Test manages a single test (created with JSLitmus.test())
   */
  function Test(name, f) {
    var test = this;

    // Test instances get EventEmitter API
    EventEmitter.call(test);

    if (!f) throw new Error('Undefined test function');
    if (!/function[^\(]*\(([^,\)]*)/.test(f)) {
      throw new Error('"' + name + '" test: Invalid test function');
    }

    // If the test function takes an argument, we assume it does the iteration
    // for us
    var isLoop = !!RegExp.$1;

    /**
     * Reset test state
     */
    function reset() {
      delete test.count;
      delete test.time;
      delete test.running;
      test.emit('reset', test);
      return test;
    }

    function clone() {
      var test = extend(new Test(name, f), test);
      return test.reset();
    }

    /**
     * Run the test n times, and use the best results
     */
    function bestOf(n) {
      var best = null;
      while (n--) {
        var t = clone();
        t.run(null, true);
        if (!best || t.period < best.period) {
          best = t;
        }
      }
      extend(test, best);
    }

    /**
     * Start running a test.  Default is to run the test asynchronously (via
     * setTimeout).  Can be made synchronous by passing true for 2nd param
     */
    function run(count, synchronous) {
      count = count || test.INIT_COUNT;
      test.running = true;

      if (synchronous) {
        _run(count, synchronous);
      } else {
        setTimeout(function() {
          _run(count);
        }, 1);
      }
      return test;
    }

    /**
     * Run, for real
     */
    function _run(count, noTimeout) {

      try {
        var start, f = test.f, now, i = count;

        // Start the timer
        start = new Date();

        // Run the test code
        test.count = count;
        test.time = 0;
        test.period = 0;

        test.emit('start', test);

        if (isLoop) {
          // Test code does it's own iteration
          f(count);
        } else {
          // Do the iteration ourselves
          while (i--) f();
        }

        // Get time test took (in secs)
        test.time = Math.max(1,new Date() - start)/1000;

        // Store iteration count and per-operation time taken
        test.count = count;
        test.period = test.time/count;

        // Do we need to keep running?
        test.running = test.time < test.MIN_TIME;

        // Publish results
        test.emit('results', test);

        // Set up for another run, if needed
        if (test.running) {
          // Use an iteration count that will (we hope) get us close to the
          // MAX_COUNT time.
          var x = test.MIN_TIME/test.time;
          var pow = Math.pow(2, Math.max(1, Math.ceil(Math.log(x)/Math.log(2))));
          count *= pow;
          if (count > test.MAX_COUNT) {
            throw new Error('Max count exceeded.  If this test uses a looping function, make sure the iteration loop is working properly.');
          }

          if (noTimeout) {
            _run(count, noTimeout);
          } else {
            run(count);
          }
        } else {
          test.emit('complete', test);
        }
      } catch (err) {
        log(err);
        // Exceptions are caught and displayed in the test UI
        test.emit('error', err);
      }

      return test;
    }

    /**
    * Get the number of operations per second for this test.
    *
    * @param normalize if true, iteration loop overhead taken into account.
    *                  Note that normalized tests may return Infinity if the
    *                  test time is of the same order as the calibration time.
    */
    function getHz(normalize) {
      var p = test.period;

      // Adjust period based on the calibration test time
      if (normalize) {
        var cal = test.isLoop ? Test.LOOP_CAL : Test.NOLOOP_CAL;
        if (!cal.period) {
          // Run calibration if needed
          cal.MIN_TIME = .3;
          cal.bestOf(3);
        }

        // Subtract off calibration time.  In theory this should never be
        // negative, but in practice the calibration times are affected by a
        // variety of factors so just clip to zero and let users test for
        // getHz() == Infinity
        p = Math.max(0, p - cal.period);
      }

      return sig(1/p, 4);
    }

    // Set properties that are specific to this instance
    extend(test, {
      // Test name
      name: name,

      // Test function
      f: f,

      // True if the test function does it's own looping (i.e. takes an arg)
      isLoop: isLoop,

      clone: clone,
      run: run,
      bestOf: bestOf,
      getHz: getHz,
      reset: reset
    });

    // IE7 doesn't do 'toString' or 'toValue' in object enumerations, so set
    // it explicitely here.
    test.toString = function() {
      if (this.time) {
        return this.name + ', f = '  +
        humanize(this.getHz()) + 'hz (' +
        humanize(this.count) + '/' + humanize(this.time) + 'secs)';
      } else {
        return this.name + ', count = '  + humanize(this.count);
      }
    };
  };

  // Set static properties
  extend(Test, {
    LOOP_CAL: new Test('loop cal', function(count) {while (count--) {}}),
    NOLOOP_CAL: new Test('noloop cal', nilf)
  });

  // Set default property values
  extend(Test.prototype, {
    // Initial number of iterations
    INIT_COUNT: 10,

    // Max iterations allowed (used to detect bad looping functions)
    MAX_COUNT: 1e9,

    // Minimum time test should take to get valid results (secs)
    MIN_TIME: 1.0
  });

  //
  // jslitmus
  //

  // Set up jslitmus context
  var jslitmus;
  if (platform.name == 'node') {
    jslitmus = exports;
  } else {
    jslitmus = root.jslitmus = {};
  }

  var tests = [], // test store (all tests added w/ jslitmus.test())
      queue = [], // test queue (to be run)
      currentTest; // currently running test

  // jslitmus gets EventEmitter API
  EventEmitter.call(jslitmus);

  /**
    * Create a new test
    */
  function test(name, f) {
    // Create the Test object
    var test = new Test(name, f);
    tests.push(test);

    // Run the next test if this one finished
    test.on('*', function() {
      // Forward test events to jslitmus listeners
      var args = Array.prototype.slice.call(arguments);
      args.unshift(test._emitting);
      jslitmus.emit.apply(jslitmus, args);

      // Auto-run the next test
      if (test._emitting == 'complete') {
        currentTest = null;
        _nextTest();
      }
    });

    jslitmus.emit('added', test);

    return test;
  }

  /**
    * Add all tests to the run queue
    */
  function runAll(e) {
    forEach(tests, _queueTest);
  }

  /**
    * Remove all tests from the run queue.  The current test has to finish on
    * it's own though
    */
  function stop() {
    while (queue.length) {
      var test = queue.shift();
    }
  }

  /**
    * Run the next test in the run queue
    */
  function _nextTest() {
    if (!currentTest) {
      var test = queue.shift();
      if (test) {
        currentTest = test;
        test.run();
      } else {
        jslitmus.emit('all_complete');
      }
    }
  }

  /**
    * Add a test to the run queue
    */
  function _queueTest(test) {
    if (indexOf(queue, test) >= 0) return;
    queue.push(test);
    _nextTest();
  }

  function clearAll() {
	tests = [];
  }

  /**
    * Generate a Google Chart URL that shows the data for all tests
    */
  function getGoogleChart(normalize) {
    var chart_title = [
      'Operations/second on ' + platform.name,
      '(' + platform.version + ' / ' + platform.os + ')'
    ];

    var n = tests.length, markers = [], data = [];
    var d, min = 0, max = -1e10;

    // Gather test data

    var markers = map(tests, function(test, i) {
      if (test.count) {
        var hz = test.getHz();
        var v = hz != Infinity ? hz : 0;
        data.push(v);
        var label = test.name + '(' + humanize(hz)+ ')';
        var marker = 't' + escape2(label) + ',000000,0,' + i + ',10';
        max = Math.max(v, max);

        return marker;
      }
    });

    if (markers.length <= 0) return null;

    // Build labels
    var labels = [humanize(min), humanize(max)];

    var w = 250, bw = 15;
    var bs = 5;
    var h = markers.length*(bw + bs) + 30 + chart_title.length*20;

    var params = {
      chtt: escape(chart_title.join('|')),
      chts: '000000,10',
      cht: 'bhg',                     // chart type
      chd: 't:' + data.join(','),     // data set
      chds: min + ',' + max,          // max/min of data
      chxt: 'x',                      // label axes
      chxl: '0:|' + labels.join('|'), // labels
      chsp: '0,1',
      chm: markers.join('|'),         // test names
      chbh: [bw, 0, bs].join(','),    // bar widths
      // chf: 'bg,lg,0,eeeeee,0,eeeeee,.5,ffffff,1', // gradient
      chs: w + 'x' + h
    };

    var url = 'http://chart.apis.google.com/chart?' + join(params);

    return url;
  }

  // Public API
  extend(jslitmus, {
    Test: Test,
    platform: platform,
    test: test,
    runAll: runAll,
    getGoogleChart: getGoogleChart,
	clearAll: clearAll
  });

  // Expose code goodness we've got here, since it's useful, but do so in a way
  // that doesn't commit us to supporting it in future versions.
  jslitmus.unsupported = {
    nilf: nilf,
    log: log,
    extend: extend,
    forEach: forEach,
    filter: filter,
    map: map,
    indexOf: indexOf,
    escape2: escape2,
    join: join,
    split: split,
    sig: sig,
    humanize: humanize
  };
})();
