const assert = require('assert');
const common = require('../lib/common.js');

describe('contentType tests',function(){

  describe('json types',function(){
    it('should match application/json',function(){
        assert(common.doContentType(['application/json'],'json'));
    });
    it('should match application/hal+json',function(){
        assert(common.doContentType(['application/hal+json'],'json'));
    });
    it('should match application/ld+json',function(){
        assert(common.doContentType(['application/ld+json'],'json'));
    });
    it('should match application/x-www-form-urlencoded',function(){
        assert(common.doContentType(['application/json-patch+json'],'json'));
    });
    it('should match application/json; charset=utf-8',function(){
        assert(common.doContentType(['application/json; charset=utf-8'],'json'));
    });
    it('should match application/json;charset=UTF-8',function(){
        assert(common.doContentType(['application/json;charset=UTF-8'],'json'));
    });
    it('should match text/json',function(){
        assert(common.doContentType(['text/json'],'json'));
    });
    it('should not match application/yaml',function(){
        assert(!common.doContentType(['application/yaml'],'json'));
    });
    it('should not match text/plain',function(){
        assert(!common.doContentType(['text/plain'],'json'));
    });
  });

  describe('xml tests',function(){
    it('should match application/xml',function(){
        assert(common.doContentType(['application/xml'],'xml'));
    });
    it('should match application/xml; charset=utf-8',function(){
        assert(common.doContentType(['application/xml; charset=utf-8'],'xml'));
    });
    it('should match text/xml',function(){
        assert(common.doContentType(['text/xml'],'xml'));
    });
    it('should match image/svg+xml',function(){
        assert(common.doContentType(['image/svg+xml'],'xml'));
    });
    it('should match application/rss+xml',function(){
        assert(common.doContentType(['application/rss+xml'],'xml'));
    });
    it('should match application/rdf+xml',function(){
        assert(common.doContentType(['application/rdf+xml'],'xml'));
    });
    it('should match application/atom+xml',function(){
        assert(common.doContentType(['application/atom+xml'],'xml'));
    });
    it('should match application/mathml+xml',function(){
        assert(common.doContentType(['application/mathml+xml'],'xml'));
    });
    it('should match application/hal+xml',function(){
        assert(common.doContentType(['application/hal+xml'],'xml'));
    });
    it('should not match text/plain',function(){
        assert(!common.doContentType(['text/plain'],'xml'));
    });
    it('should not match application/json',function(){
        assert(!common.doContentType(['application/json'],'xml'));
    });
  });

  describe('yaml tests',function(){
    it('should match application/x-yaml',function(){
        assert(common.doContentType(['application/x-yaml'],'yaml'));
    });
    it('should match text/x-yaml',function(){
        assert(common.doContentType(['text/x-yaml'],'yaml'));
    });
    it('should not match text/plain',function(){
        assert(!common.doContentType(['text/plain'],'yaml'));
    });
    it('should not match application/xml',function(){
        assert(!common.doContentType(['application/xml'],'yaml'));
    });
  });

  describe('form tests',function(){
    it('should match multipart/form-data',function(){
        assert(common.doContentType(['multipart/form-data'],'form'));
    });
    it('should match application/x-www-form-urlencoded',function(){
        assert(common.doContentType(['application/x-www-form-urlencoded'],'form'));
    });
    it('should match application/octet-stream',function(){
        assert(common.doContentType(['application/octet-stream'],'form'));
    });
    it('should not match text/plain',function(){
        assert(!common.doContentType(['text/plain'],'form'));
    });
  });

});

describe('array tests',function(){

  describe('positive tests',function(){
    it('should match application/json and another type',function(){
        assert(common.doContentType(['application/json','text/plain'],'json'));
    });
    it('should match another type and application/json',function(){
        assert(common.doContentType(['text/plain','application/json'],'json'));
    });
  });
  describe('negative tests',function(){
    it('should not match two other types',function(){
        assert(!common.doContentType(['text/plain','image/jpeg'],'json'));
    });
    it('should not match an empty array',function(){
        assert(!common.doContentType([],'json'));
    });
    it('should not match an unknown format',function(){
        assert(!common.doContentType(['application/octet-stream'],'file'));
    });
  });
});

