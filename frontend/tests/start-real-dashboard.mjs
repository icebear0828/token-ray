import { spawn } from 'node:child_process';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const repoRoot = path.resolve(__dirname, '../..');
const dataDir = path.join(os.tmpdir(), 'token-ray-playwright');
const goCache = path.join(os.tmpdir(), 'token-ray-go-cache');

fs.rmSync(dataDir, { recursive: true, force: true });
fs.mkdirSync(dataDir, { recursive: true });
fs.mkdirSync(goCache, { recursive: true });

const child = spawn('go', [
  'run',
  './cmd/dashboard',
  '--web',
  '--lan=false',
  '--sync-mode=once',
  '--data-dir',
  dataDir,
  '--port',
  '19183',
], {
  cwd: repoRoot,
  env: {
    ...process.env,
    GOCACHE: goCache,
  },
  stdio: 'inherit',
});

const stop = () => {
  if (!child.killed) {
    child.kill('SIGTERM');
  }
};

process.on('SIGINT', stop);
process.on('SIGTERM', stop);
process.on('exit', stop);

child.on('exit', (code, signal) => {
  if (signal) {
    process.exit(0);
  }
  process.exit(code ?? 1);
});
