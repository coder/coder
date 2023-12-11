require("dot").process({
	global: "_page.render"
	, destination: __dirname + "/render/"
	, path: (__dirname + "/../templates")
});

var express = require('express')
, http = require('http')
, app = express()
, render = require('./render')
;

app.get('/', function(req, res){
  res.send(render.dashboard({text:"Good morning!"}));
});

app.use(function(err, req, res, next) {
	console.error(err.stack);
	res.status(500).send('Something broke!');
});

var httpServer = http.createServer(app);
httpServer.listen(3000, function() {
	console.log('Listening on port %d', httpServer.address().port);
});
