import assert from 'node:assert/strict';
import { execFile } from 'node:child_process';
import { mkdir, mkdtemp, readFile, readdir, rm, writeFile } from 'node:fs/promises';
import { join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { promisify } from 'node:util';
import test from 'node:test';

const execute = promisify(execFile);
const root = fileURLToPath(new URL('../template/', import.meta.url));

test('template is self-contained and uses the agent-owned Go manager', async () => {
  const template = await readFile(join(root, 'main.tf'), 'utf8');
  const dockerfile = await readFile(join(root, 'image/Dockerfile'), 'utf8');
  const startup = await readFile(join(root, 'image/startup.sh'), 'utf8');
  const files = await readdir(join(root, 'image'));
  assert.deepEqual(files.sort(), ['Dockerfile', 'startup.sh']);
  assert.doesNotMatch(dockerfile, /COPY adapter|npm ci|startup\.mjs|shutdown\.mjs/);
  assert.doesNotMatch(startup, /node |coder-acp|\/restore|\/prompt|\/shutdown/);
  assert.match(startup, /exec coder native-agent configure --setup/);
  assert.match(template, /exec sh \/opt\/native-agent\/startup\.sh/);
  assert.match(template, /exec coder native-agent shutdown/);
  const container = template.split('resource "docker_container" "workspace" {')[1];
  assert.match(container, /"CODER_AGENT_NATIVE_AGENTS=true"/);
  assert.match(container, /"BINARY_DIR=\/opt\/coder\/bin"/);
  assert.match(template, /PATH\s*= "\/opt\/coder\/bin:/);
  assert.match(dockerfile, /PATH=\/opt\/coder\/bin:/);
  assert.match(dockerfile, /BINARY_DIR=\/opt\/coder\/bin/);
  assert.match(template, /start_blocks_login\s*= true/);
  assert.match(template, /run_on_stop\s*= true/);
  assert.match(template, /destroy_grace_seconds\s*= 90/);
  assert.match(dockerfile, /FROM node:22-bookworm-slim/);
  assert.match(dockerfile, /@anthropic-ai\/claude-code@2\.1\.309/);
  assert.match(dockerfile, /@openai\/codex@0\.164\.0/);
  assert.match(dockerfile, /@opencode\/cli@2\.0\.28/);
  assert.match(dockerfile, /test "\$\(opencode --version\)" = "opencode v2\.0\.28"/);
  assert.match(template, /CODER_NATIVE_JOB_PROMPT_BASE64\s*= base64encode\(data\.coder_parameter\.prompt\.value\)/);
  assert.match(template, /CODER_NATIVE_JOB_REPO_BASE64\s*= base64encode\(data\.coder_parameter\.repository\.value\)/);
  assert.match(template, /read_only\s*= true/);
  assert.match(template, /container_path\s*= "\/run\/secrets\/claude-auth\.json"/);
  assert.match(template, /for_each\s*= var\.claude_auth_file != "" \? \[var\.claude_auth_file\] : \[\]/);
  assert.doesNotMatch(template, /codex_api_key|CODER_NATIVE_JOB_CODEX_API_KEY_BASE64/);
  const manifest = template.split('resource "coder_agent" "main" {')[1].split('resource "coder_script"')[0];
  assert.doesNotMatch(manifest, /(?:API_KEY|AUTH_TOKEN|file\s*\()/);
});

test('startup forwards encoded input literally and never logs credentials or launches a service', async t => {
  const home = await mkdtemp('/tmp/opencode/native-template-test-');
  t.after(() => rm(home, { recursive: true, force: true }));
  const bin = join(home, 'bin');
  await mkdir(bin);
  const prompt = '$(touch /not-a-command) `literal` $HOME ${CODER_AGENT_TOKEN}\n"quoted"';
  const encoded = Buffer.from(prompt).toString('base64');
  await writeFile(join(bin, 'coder'), `#!${process.execPath}
import assert from 'node:assert/strict';
assert.deepEqual(process.argv.slice(2), ['native-agent', 'configure', '--setup']);
assert.equal(process.env.CODER_NATIVE_JOB_PROMPT_BASE64, ${JSON.stringify(encoded)});
assert.equal(process.env.CODER_NATIVE_JOB_CODEX_API_KEY_BASE64, undefined);
assert.equal(process.umask(), 0o077);
`, { mode: 0o700 });
  const result = await execute('/bin/sh', [join(root, 'image/startup.sh')], {
    env: { ...process.env, HOME: home, PATH: bin + ':' + process.env.PATH,
      CODER_NATIVE_JOB_PROMPT_BASE64: encoded },
  });
  assert.equal(result.stdout, '');
  assert.equal(result.stderr, '');
});

test('repository parameter accepts public HTTPS URLs', async () => {
  const template = await readFile(join(root, 'main.tf'), 'utf8');
  const repository = template.split('data "coder_parameter" "repository" {')[1].split('\ndata "coder_parameter"')[0];
  const pattern = repository.match(/regex\s*=\s*("(?:[^"\\]|\\.)*")/)[1];
  const regex = new RegExp(JSON.parse(pattern));
  for (const value of ['', 'https://github.com/coder/coder', 'https://github.com/coder/agentapi.git']) {
    assert.equal(regex.test(value), true);
  }
  for (const value of ['http://github.com/coder/coder', 'git@github.com:coder/coder.git', 'https://', 'https://example.com/my repo']) {
    assert.equal(regex.test(value), false);
  }
});
