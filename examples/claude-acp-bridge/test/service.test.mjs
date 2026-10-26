import assert from 'node:assert/strict';
import { mkdtemp, readFile, rm, writeFile } from 'node:fs/promises';
import { join } from 'node:path';
import test from 'node:test';
import { startDetached } from '../src/service.mjs';

for (const options of [{ opencode: true }, { codex: true, opencode: true }]) {
  test('detached broker forwards explicit native startup flags: ' + JSON.stringify(options), async t => {
    const directory = await mkdtemp('/tmp/opencode/bridge-detach-');
    t.after(() => rm(directory, { recursive: true, force: true }));
    const cli = join(directory, 'fake.mjs');
    const argsFile = join(directory, 'args.json');
    const stateFile = join(directory, 'bridge.json');
    await writeFile(cli, `import { writeFileSync } from 'node:fs';
writeFileSync(${JSON.stringify(argsFile)}, JSON.stringify(process.argv.slice(2)));
process.send({ type: 'bridge-ready' }, () => process.disconnect());
`, { mode: 0o600 });
    const result = await startDetached(cli, stateFile, 'haiku', options);
    assert.ok(Number.isSafeInteger(result.pid));
    assert.equal(result.logFile, join(directory, 'broker.log'));
    assert.deepEqual(JSON.parse(await readFile(argsFile, 'utf8')), [
      'serve', '--state', stateFile, '--model', 'haiku',
      ...(options.codex ? ['--codex'] : []), '--opencode',
    ]);
  });
}
