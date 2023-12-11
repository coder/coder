# jgeXml - The Just-Good-Enough XML Toolkit

[![Build status](https://travis-ci.org/Mermade/jgeXml.svg?branch=master)](https://travis-ci.org/Mermade/jgeXml)
[![Join the Mermade Slack](https://img.shields.io/badge/Slack-Mermade-brightgreen)](https://join.slack.com/t/mermade/shared_invite/zt-g78g7xir-MLE_CTCcXCdfJfG3CJe9qA)

[![Share on Twitter][twitter-image]][twitter-link]
[![Follow on Twitter][twitterFollow-image]][twitterFollow-link]

jgeXml provides an event-driven parser to process XML 1.0 / 1.1. Both pull and push modes are supported. Tools are included for writing XML (documents or fragments) and to convert between XML and JSON.

The code has no dependencies on other modules or native libraries.

Setting up a push-parser is as simple as:

```javascript
const jgexml = require('jgexml');
const result = jgexml.parse(xml, function(state, token) {
  //...
});
```

## Events (stateCodes)

* `sDeclaration`
* `sDocType`
* `sDTD`
* `sElement`
* `sAttribute`
* `sValue`
* `sEndElement`
* `sContent`
* `sComment`
* `sProcessingInstruction`
* `sCData`
* `sError`
* `sEndDocument`

No event is generated for ignoreable whitespace, unlike SAX. Empty elements are normalised into sElement/sEndElement pairs.

## Notes

jgeXml is a non-validating parser. It attempts to report if the XML is well-formed or not.

Both when reading and writing, attributes follow after the element event, and in the order they are given in the source.

When converting to JSON, the attributePrefix (to avoid name clashes with child elements) is configurable per parse.

In JSON, child elements can be represented as properties (the default) or objects (exposing the parser's intermediary state).

The parser by default treats all content as strings when converting to JSON, optionally data can be coerced to primitive numbers or null values.

The `xsd2json` utility can convert most simple XML Schemas to JSON schema draft 4. XSD's may of course be converted to JSON simply as if they were XML documents too.

Experimental JSONPath and JSONT utilities are under development.

## Limitations

jgeXml is currently schema agnostic and staunchly atheist when it comes to DTDs. It can parse XML documents with schema information, but it is up to the
consumer to interpret the namespace portions of element names. It can parse internal DTDs, but does nothing with them.
`xmlWrite` minimally supports DTDs but you must build them and the DOCTYPE yourself.

The parser is string-based; to process streams, read the data into a string first. It may be memory intensive on large documents.

## CLI commands

* `xml2json` - convert XML to JSON.
* `json2xml` - convert JSON to XML.
* `xsd2json` - convert XSD to JSON Schema.

## Examples

See in the `examples` directory: `xml2xml.js` for parsing XML to XML, `fragment.js` for writing XML fragments, `jpath.js` for JSONPath examples, `jsont.js` for JSONT examples and `pullparser.js` / `pushparser.js` for how to set up and run the parser.

[twitter-image]: https://img.shields.io/twitter/url/http/PermittedSoc.svg?style=social
[twitter-link]: https://twitter.com/share?source=tweetbutton&text=jgeXml%20parser%20Via%20%40PermittedSoc&url=https%3A%2F%2Fgithub.com%2FMikeRalphson%2FjgeXml
[twitterFollow-image]: https://img.shields.io/twitter/follow/PermittedSoc.svg?style=social
[twitterFollow-link]: https://twitter.com/intent/follow?screen_name=PermittedSoc
