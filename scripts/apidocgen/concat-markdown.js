#!/usr/bin/env node

const fs = require('fs');
const path = require('path');

if (process.argv.length < 4) {
  console.error('Usage: node concat-markdown.js <input-dir> <output-file>');
  process.exit(1);
}

const inputDir = process.argv[2];
const outputFile = process.argv[3];

try {
  let combinedMarkdown = '';
  
  // Read the main README.md first
  const readmePath = path.join(inputDir, 'README.md');
  if (fs.existsSync(readmePath)) {
    const readmeContent = fs.readFileSync(readmePath, 'utf8');
    combinedMarkdown += '<!-- APIDOCGEN: BEGIN SECTION -->\n';
    combinedMarkdown += readmeContent;
    combinedMarkdown += '\n\n';
  }
  
  // Read all API files from the Apis directory
  const apisDir = path.join(inputDir, 'Apis');
  if (fs.existsSync(apisDir)) {
    const apiFiles = fs.readdirSync(apisDir)
      .filter(file => file.endsWith('.md'))
      .sort(); // Sort alphabetically for consistent output
    
    for (const apiFile of apiFiles) {
      const apiPath = path.join(apisDir, apiFile);
      const apiContent = fs.readFileSync(apiPath, 'utf8');
      
      // Add section marker for postprocessor
      combinedMarkdown += '<!-- APIDOCGEN: BEGIN SECTION -->\n';
      combinedMarkdown += apiContent;
      combinedMarkdown += '\n\n';
    }
  }
  
  // Write the combined markdown file
  fs.writeFileSync(outputFile, combinedMarkdown);
  
  console.log(`Successfully combined markdown files into ${outputFile}`);
  console.log(`Total size: ${Math.round(combinedMarkdown.length / 1024)}KB`);
  
} catch (error) {
  console.error('Error:', error.message);
  process.exit(1);
}
