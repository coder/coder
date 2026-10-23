#!/usr/bin/env node
import { cp, mkdir, readdir } from 'node:fs/promises';
import { dirname, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

const root = resolve(dirname(fileURLToPath(import.meta.url)), '..');

/** Package the experimental template with only the adapter files it executes. */
export async function packageTemplate(destination) {
  await mkdir(destination, { mode: 0o700 });
  for (const name of await readdir(join(root, 'template'))) {
    await cp(join(root, 'template', name), join(destination, name), { recursive: true, errorOnExist: true, force: false });
  }
  const adapter = join(destination, 'image', 'adapter');
  for (const path of ['package.json', 'package-lock.json', 'install.sh', 'src', 'scripts/install.mjs', 'plugin/hooks']) {
    await mkdir(dirname(join(adapter, path)), { recursive: true });
    await cp(join(root, path), join(adapter, path), { recursive: true, errorOnExist: true, force: false });
  }
  // Coder's directory uploader skips hidden directories. Restore this in the image.
  await cp(join(root, 'plugin/.claude-plugin/plugin.json'), join(adapter, 'plugin-manifest.json'), { errorOnExist: true, force: false });
}

if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  const destination = process.argv[2];
  if (!destination || process.argv.length !== 3) {
    console.error('Usage: node scripts/package-template.mjs NEW_DIRECTORY');
    process.exitCode = 1;
  } else {
    await packageTemplate(resolve(destination)).then(
      () => console.log('Packaged native agent template at ' + resolve(destination)),
      () => { console.error('Template packaging failed. Use a new destination directory and check the source files.'); process.exitCode = 1; },
    );
  }
}
