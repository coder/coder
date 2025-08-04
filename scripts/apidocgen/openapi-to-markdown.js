#!/usr/bin/env node

const fs = require('fs');
const path = require('path');

// Read the OpenAPI spec
function readOpenAPISpec(filePath) {
  const content = fs.readFileSync(filePath, 'utf8');
  return JSON.parse(content);
}

// Group paths by tags
function groupPathsByTags(spec) {
  const groups = {};
  
  for (const [path, pathItem] of Object.entries(spec.paths || {})) {
    for (const [method, operation] of Object.entries(pathItem)) {
      if (typeof operation !== 'object' || !operation.tags) continue;
      
      const tag = operation.tags[0] || 'General';
      if (!groups[tag]) {
        groups[tag] = [];
      }
      
      groups[tag].push({
        path,
        method: method.toUpperCase(),
        operation,
        summary: operation.summary || '',
        description: operation.description || ''
      });
    }
  }
  
  return groups;
}

// Generate markdown for a single operation
function generateOperationMarkdown(op) {
  let md = `## ${op.method} ${op.path}\n\n`;
  
  if (op.summary) {
    md += `${op.summary}\n\n`;
  }
  
  if (op.description) {
    md += `${op.description}\n\n`;
  }
  
  // Parameters
  if (op.operation.parameters && op.operation.parameters.length > 0) {
    md += `### Parameters\n\n`;
    md += `| Name | In | Type | Required | Description |\n`;
    md += `|------|----|----- |----------|-------------|\n`;
    
    for (const param of op.operation.parameters) {
      const name = param.name || '';
      const location = param.in || '';
      const type = param.schema?.type || param.type || '';
      const required = param.required ? 'Yes' : 'No';
      const description = param.description || '';
      
      md += `| ${name} | ${location} | ${type} | ${required} | ${description} |\n`;
    }
    md += `\n`;
  }
  
  // Request body
  if (op.operation.requestBody) {
    md += `### Request Body\n\n`;
    const content = op.operation.requestBody.content;
    if (content) {
      for (const [mediaType, schema] of Object.entries(content)) {
        md += `**${mediaType}**\n\n`;
        if (schema.schema) {
          md += `\`\`\`json\n${JSON.stringify(schema.example || {}, null, 2)}\n\`\`\`\n\n`;
        }
      }
    }
  }
  
  // Responses
  if (op.operation.responses) {
    md += `### Responses\n\n`;
    md += `| Status | Description |\n`;
    md += `|--------|-------------|\n`;
    
    for (const [status, response] of Object.entries(op.operation.responses)) {
      const description = response.description || '';
      md += `| ${status} | ${description} |\n`;
    }
    md += `\n`;
  }
  
  // Example curl command
  md += `### Example\n\n`;
  md += `\`\`\`shell\n`;
  md += `curl -X ${op.method} \\\n`;
  md += `  "https://coder.example.com/api/v2${op.path}" \\\n`;
  md += `  -H "Coder-Session-Token: <your-token>"\n`;
  md += `\`\`\`\n\n`;
  
  return md;
}

// Generate markdown for a tag group
function generateTagMarkdown(tag, operations) {
  let md = `<!-- APIDOCGEN: BEGIN SECTION -->\n`;
  md += `# ${tag}\n\n`;
  
  // Add a description for common tags
  const tagDescriptions = {
    'General': 'General API information and basic operations.',
    'Authentication': 'Authentication and authorization endpoints.',
    'Users': 'User management and profile operations.',
    'Workspaces': 'Workspace creation, management, and operations.',
    'Templates': 'Template management and operations.',
    'Organizations': 'Organization management and settings.',
    'Agents': 'Workspace agent management and operations.',
    'Builds': 'Workspace build operations and status.',
    'Files': 'File upload and download operations.',
    'Git': 'Git integration and repository operations.',
    'Audit': 'Audit log and security operations.',
    'Debug': 'Debug and troubleshooting endpoints.',
    'Enterprise': 'Enterprise-specific features and operations.',
    'Insights': 'Analytics and usage insights.',
    'Members': 'Organization member management.',
    'Notifications': 'Notification and alert management.',
    'Provisioning': 'Infrastructure provisioning operations.',
    'Prebuilds': 'Prebuild management and operations.',
    'WorkspaceProxies': 'Workspace proxy configuration and management.',
    'PortSharing': 'Port sharing and forwarding operations.',
    'Schemas': 'API schema definitions and validation.'
  };
  
  if (tagDescriptions[tag]) {
    md += `${tagDescriptions[tag]}\n\n`;
  }
  
  // Sort operations by path and method
  operations.sort((a, b) => {
    if (a.path !== b.path) return a.path.localeCompare(b.path);
    return a.method.localeCompare(b.method);
  });
  
  for (const operation of operations) {
    md += generateOperationMarkdown(operation);
  }
  
  return md;
}

// Main function
function main() {
  const args = process.argv.slice(2);
  if (args.length < 2) {
    console.error('Usage: node openapi-to-markdown.js <input-file> <output-file>');
    process.exit(1);
  }
  
  const inputFile = args[0];
  const outputFile = args[1];
  
  try {
    console.log(`Reading OpenAPI spec from ${inputFile}`);
    const spec = readOpenAPISpec(inputFile);
    
    console.log('Grouping paths by tags...');
    const groups = groupPathsByTags(spec);
    
    console.log(`Found ${Object.keys(groups).length} tag groups`);
    
    let allMarkdown = '';
    
    // Generate markdown for each tag group
    const tagOrder = ['General', 'Authentication', 'Users', 'Workspaces', 'Templates', 'Organizations'];
    const processedTags = new Set();
    
    // Process tags in preferred order first
    for (const tag of tagOrder) {
      if (groups[tag]) {
        console.log(`Generating markdown for ${tag} (${groups[tag].length} operations)`);
        allMarkdown += generateTagMarkdown(tag, groups[tag]);
        processedTags.add(tag);
      }
    }
    
    // Process remaining tags alphabetically
    const remainingTags = Object.keys(groups)
      .filter(tag => !processedTags.has(tag))
      .sort();
      
    for (const tag of remainingTags) {
      console.log(`Generating markdown for ${tag} (${groups[tag].length} operations)`);
      allMarkdown += generateTagMarkdown(tag, groups[tag]);
    }
    
    console.log(`Writing markdown to ${outputFile}`);
    fs.writeFileSync(outputFile, allMarkdown);
    
    console.log('Markdown generation complete!');
    
  } catch (error) {
    console.error('Error:', error.message);
    process.exit(1);
  }
}

if (require.main === module) {
  main();
}
