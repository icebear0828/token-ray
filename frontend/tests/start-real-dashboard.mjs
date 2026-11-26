import { spawn } from 'node:child_process';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

function resolvePort(argv, envPort) {
  for (let i = 0; i < argv.length; i += 1) {
    const arg = argv[i];
    if (arg === '--port' && argv[i + 1]) {
      return argv[i + 1];
    }
    if (arg.startsWith('--port=')) {
      return arg.slice('--port='.length);
    }
  }
  return envPort ?? '19183';
}

const port = resolvePort(process.argv.slice(2), process.env.PW_PORT);
const __dirname = path.dirname(fileURLToPath(import.meta.url));
const repoRoot = path.resolve(__dirname, '../..');
const dataDir = path.join(os.tmpdir(), 'ai-flight-dashboard-playwright-' + port);
const goCache = path.join(os.tmpdir(), 'ai-flight-dashboard-go-cache');

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
  port,
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
