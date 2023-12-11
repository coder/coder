'use strict';

var jsont = require('../jsont');

function run(obj,rules) {
	console.log();
	console.log(obj);
	console.log('+');
	console.log(rules);
	console.log('=');
	console.log(jsont.transform(obj,rules));	
}

// see http://goessner.net/articles/jsont/

var obj = { "link": {"uri":"http://company.com", "title":"company homepage" }};
var rules = { "link": "<a href=\"{link.uri}\">{link.title}</a>" };
run(obj,rules);

obj = { "line": { "p1": {"x":2, "y":3},
            "p2": {"x":4, "y":5} }};
rules = { "self": "<svg>{line}</svg>",
  "line": "<line x1=\"{$.p1.x}\" y1=\"{$.p1.y}\"" +
                "x2=\"{$.p2.x}\" y2=\"{$.p2.y}\" />" };
run(obj,rules);

obj = ["red", "green", "blue"]
rules = {"self": "<ul>\n{$}</ul>",
	"self[*]": "  <li>{$}</li>\n"};
run(obj,rules);

obj = { "color": "blue",
  "closed": true,
  "points": [[10,10],[20,10],[20,20],[10,20]] };
rules = { "self": "<svg><{closed} stroke=\"{color}\" points=\"{points}\" />"+
          "</svg>",
  "closed": function(x){return x ? "polygon" : "polyline";}, 
  "points[*][*]": "{$} " };
run(obj,rules);
