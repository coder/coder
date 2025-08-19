const ReactMarkdown = require("react-markdown");
const gfm = require("remark-gfm");
const React = require("react");

// Test markdown with GFM alert containing a link
const testMarkdown = `> [!NOTE]
> This template is centrally managed by CI/CD in the [coder/templates](https://github.com/coder/templates) repository.`;

console.log("Testing GFM alert parsing with link...");
console.log("Markdown:", testMarkdown);
