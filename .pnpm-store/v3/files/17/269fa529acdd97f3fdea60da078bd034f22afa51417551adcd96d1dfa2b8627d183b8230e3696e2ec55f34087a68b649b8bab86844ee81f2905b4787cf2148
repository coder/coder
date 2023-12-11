(function() {
	var doT = require('../doT.js'),
		fs = require('fs'),
		data = { name: "Foo", f1: 1, f2: 2, f3: 3, altEmail: "conditional works", farray:[{farray:[1,2,3,[11,22,33]],person:{name:'Ell',age:23}},{farray:{how:'really'}}, {farray:[5,6,7,8]}]},
		defs = { a: 100, b: 200};

	defs.loadfile = function(path) {
		return fs.readFileSync(process.argv[1].replace(/\/[^\/]*$/,path));
	};
	defs.externalsnippet = defs.loadfile('/snippet.txt');

	fs.readFile(process.argv[1].replace(/\/[^\/]*$/,'/advancedsnippet.txt'), function (err, snippet) {
		if (err) {
			console.log("Error reading advancedsnippet.txt " + err);
		} else {
			var doTCompiled = doT.template(snippet.toString(), undefined, defs);
			console.log("Generated function: \n" + doTCompiled.toString());
			console.log("Result of calling with " + JSON.stringify(data) + " :\n" + doTCompiled(data));
		}
	});
}());
