import assert from 'node:assert/strict';
import fs from 'node:fs';
import test from 'node:test';

test('Test workflow reruns PR release gate when release metadata changes', () => {
  const workflow = fs.readFileSync('.github/workflows/test.yml', 'utf8');

  assert.match(workflow, /pull_request:\n\s+branches: \[main\]\n\s+types: \[([^\]]+)\]/);

  const [, rawTypes] = workflow.match(/pull_request:\n\s+branches: \[main\]\n\s+types: \[([^\]]+)\]/) ?? [];
  const types = rawTypes.split(',').map((type) => type.trim());

  for (const requiredType of ['opened', 'synchronize', 'reopened', 'edited', 'labeled', 'unlabeled']) {
    assert.ok(types.includes(requiredType), `missing pull_request type: ${requiredType}`);
  }
});

test('Release workflow updates release notes without relying on git checkout context', () => {
  const workflow = fs.readFileSync('.github/workflows/release.yml', 'utf8');

  assert.match(
    workflow,
    /gh release edit "\$GITHUB_REF_NAME" --repo "\$GITHUB_REPOSITORY" --title "\$GITHUB_REF_NAME" --notes-file release-notes\.md/,
  );
});

test('Release workflow builds Wails binaries with -trimpath', () => {
  const workflow = fs.readFileSync('.github/workflows/release.yml', 'utf8');
  const buildLines = workflow.match(/^\s*wails build .+$/gm) ?? [];

  assert.equal(buildLines.length, 4, `expected 4 wails build commands, got ${buildLines.length}`);
  for (const line of buildLines) {
    assert.match(line, /\s-trimpath(?:\s|$)/, `expected -trimpath in ${line.trim()}`);
  }
});
