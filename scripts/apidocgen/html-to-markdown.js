#!/usr/bin/env node

const fs = require('fs');
const TurndownService = require('turndown');

if (process.argv.length < 4) {
  console.error('Usage: node html-to-markdown.js <input.html> <output.md>');
  process.exit(1);
}

const inputFile = process.argv[2];
const outputFile = process.argv[3];

try {
  // Read HTML file
  const html = fs.readFileSync(inputFile, 'utf8');
  
  // Configure turndown
  const turndownService = new TurndownService({
    headingStyle: 'atx',
    codeBlockStyle: 'fenced'
  });
  
  // Convert HTML to markdown
  let markdown = turndownService.turndown(html);
  
  // Add section separators for the postprocessor
  // Split by main headings and add section markers
  const sections = markdown.split(/^# /m);
  let processedMarkdown = '';
  
  for (let i = 1; i < sections.length; i++) {
    processedMarkdown += '<!-- APIDOCGEN: BEGIN SECTION -->\n';
    processedMarkdown += '# ' + sections[i];
    if (i < sections.length - 1) {
      processedMarkdown += '\n\n';
    }
  }
  
  // Write markdown file
  fs.writeFileSync(outputFile, processedMarkdown);
  
  console.log(`Successfully converted ${inputFile} to ${outputFile}`);
  
} catch (error) {
  console.error('Error:', error.message);
  process.exit(1);
}
