import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';
import { globSync } from 'node:fs';

const babel = await import('./node_modules/.pnpm/@babel+core@7.28.5/node_modules/@babel/core/lib/index.js');
const syntaxTSPlugin = './node_modules/.pnpm/@babel+plugin-syntax-typescript@7.24.7_@babel+core@7.28.5/node_modules/@babel/plugin-syntax-typescript/lib/index.js';

// Scan all files in the compiled directories
const patterns = [
  'src/pages/AgentsPage/**/*.tsx',
  'src/pages/AgentsPage/**/*.ts',
  'src/components/ai-elements/**/*.tsx',
  'src/components/ai-elements/**/*.ts',
];

const allFiles = [];
for (const pattern of patterns) {
  const { globSync } = await import('node:fs');
  // Use a simple find approach instead
}

// Just use child_process to glob
import { execSync } from 'node:child_process';
const files = execSync(
  "find src/pages/AgentsPage src/components/ai-elements -type f \\( -name '*.tsx' -o -name '*.ts' \\) ! -name '*.test.*' ! -name '*.stories.*' ! -name '*.jest.*'",
  { encoding: 'utf-8' }
).trim().split('\n').filter(Boolean);

let totalCompiled = 0;
let totalDiagnostics = 0;
const failures = [];

for (const file of files) {
  const code = readFileSync(file, 'utf-8');
  const isTSX = file.endsWith('.tsx');
  const diagnostics = [];

  try {
    const result = babel.transformSync(code, {
      plugins: [
        [syntaxTSPlugin, { isTSX }],
        ['babel-plugin-react-compiler', {
          logger: {
            logEvent(filename, event) {
              if (event.kind === 'CompileError' || event.kind === 'CompileSkip') {
                const msg = event.detail || event.reason || '';
                // Extract just the error type
                const short = typeof msg === 'string' ? msg.replace(/^Error: /, '').split('.')[0].split('(http')[0].trim() : String(msg);
                diagnostics.push({
                  line: event.fnLoc?.start?.line,
                  short,
                });
              }
            },
          },
        }],
      ],
      filename: file,
    });

    const slots = (result.code.match(/const \$ = _c\(\d+\)/g) || []);
    totalCompiled += slots.length;

    if (diagnostics.length) {
      totalDiagnostics += diagnostics.length;
      // Dedupe by line+message
      const seen = new Set();
      const unique = diagnostics.filter(d => {
        const key = `${d.line}:${d.short}`;
        if (seen.has(key)) return false;
        seen.add(key);
        return true;
      });
      failures.push({ file, compiled: slots.length, diagnostics: unique });
    }
  } catch (e) {
    failures.push({ file, compiled: 0, diagnostics: [{ line: 0, short: `Transform error: ${String(e.message).substring(0, 120)}` }] });
  }
}

console.log(`\nTotal: ${totalCompiled} functions compiled across ${files.length} files`);
console.log(`Files with diagnostics: ${failures.length}\n`);

for (const f of failures) {
  const short = f.file.replace('src/pages/AgentsPage/', '').replace('src/components/ai-elements/', 'ai/');
  console.log(`✗ ${short} (${f.compiled} compiled)`);
  for (const d of f.diagnostics) {
    console.log(`    line ${d.line}: ${d.short}`);
  }
}

if (failures.length === 0) {
  console.log('✓ All files compile cleanly.');
}
