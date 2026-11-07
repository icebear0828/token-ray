import assert from 'node:assert/strict';
import test from 'node:test';

import {
  convertUpstreamPrice,
  normalizeModelKey,
  isRelevantModel,
  mergePricingTables,
  formatPricingTableJSON,
} from './sync_pricing.mjs';

test('convertUpstreamPrice correctly scales per-token costs to per-million', () => {
  const raw = {
    input_cost_per_token: 0.000003,
    output_cost_per_token: 0.000015,
    cache_read_input_token_cost: 0.0000003,
    cache_creation_input_token_cost: 0.00000375,
  };

  const price = convertUpstreamPrice(raw);
  assert.equal(price.input_price_per_m, 3.0);
  assert.equal(price.output_price_per_m, 15.0);
  assert.equal(price.cached_price_per_m, 0.3);
  assert.equal(price.cache_creation_price_per_m, 3.75);
});

test('normalizeModelKey strips provider prefixes and normalizes separators', () => {
  assert.equal(normalizeModelKey('anthropic/claude-3-7-sonnet-20250219'), 'claude-3-7-sonnet-20250219');
  assert.equal(normalizeModelKey('gemini/gemini-2.5-pro'), 'gemini-2.5-pro');
  assert.equal(normalizeModelKey('openai/gpt-4o'), 'gpt-4o');
  assert.equal(normalizeModelKey('deepseek/deepseek-chat'), 'deepseek-chat');
});

test('isRelevantModel only accepts supported LLM families and rejects noise', () => {
  assert.equal(isRelevantModel('claude-3-7-sonnet', { mode: 'chat', input_cost_per_token: 0.000003 }), true);
  assert.equal(isRelevantModel('gemini-2.5-flash', { mode: 'chat', input_cost_per_token: 0.0000001 }), true);
  assert.equal(isRelevantModel('deepseek-chat', { mode: 'chat', input_cost_per_token: 0.0000002 }), true);
  assert.equal(isRelevantModel('text-embedding-3-small', { mode: 'embedding' }), false);
  assert.equal(isRelevantModel('rerank-english-v3.0', { mode: 'rerank' }), false);
  assert.equal(isRelevantModel('random-unknown-model-xyz', { mode: 'chat' }), false);
});

test('mergePricingTables merges new models while preserving existing overrides', () => {
  const current = {
    models: {
      'claude-3-7-sonnet-20250219': {
        input_price_per_m: 3.0,
        cached_price_per_m: 0.3,
        cache_creation_price_per_m: 3.75,
        output_price_per_m: 15.0,
      },
      'gemini-3.5-flash': {
        input_price_per_m: 1.5,
        cached_price_per_m: 0.15,
        cache_creation_price_per_m: 1.5,
        output_price_per_m: 9.0,
      },
    },
  };

  const upstream = {
    'claude-3-7-sonnet-20250219': {
      input_cost_per_token: 0.000003,
      output_cost_per_token: 0.000015,
      cache_read_input_token_cost: 0.0000003,
      mode: 'chat',
    },
    'deepseek/deepseek-v3.2': {
      input_cost_per_token: 0.00000028,
      output_cost_per_token: 0.00000042,
      cache_read_input_token_cost: 0.000000028,
      mode: 'chat',
    },
  };

  const { merged, addedCount, updatedCount } = mergePricingTables(current, upstream, { overwriteExisting: false });

  assert.equal(addedCount, 1);
  assert.equal(updatedCount, 0);
  assert.ok(merged.models['deepseek-v3.2']);
  assert.equal(merged.models['deepseek-v3.2'].input_price_per_m, 0.28);
  assert.equal(merged.models['deepseek-v3.2'].output_price_per_m, 0.42);

  // Existing claude model prices are preserved
  assert.equal(merged.models['claude-3-7-sonnet-20250219'].cache_creation_price_per_m, 3.75);
  assert.equal(merged.models['claude-3-7-sonnet-20250219'].input_price_per_m, 3.0);
});

test('formatPricingTableJSON formats sorted keys with 2-space indentation', () => {
  const data = {
    models: {
      'zebra-model': { input_price_per_m: 1, cached_price_per_m: 0, cache_creation_price_per_m: 0, output_price_per_m: 2 },
      'alpha-model': { input_price_per_m: 1, cached_price_per_m: 0, cache_creation_price_per_m: 0, output_price_per_m: 2 },
    },
  };

  const jsonStr = formatPricingTableJSON(data);
  const alphaIdx = jsonStr.indexOf('"alpha-model"');
  const zebraIdx = jsonStr.indexOf('"zebra-model"');
  assert.ok(alphaIdx > 0 && zebraIdx > alphaIdx);
  assert.ok(jsonStr.endsWith('\n'));
});

test('sync-pricing workflow file has valid structure and required bilingual sections', async () => {
  const fs = await import('node:fs');
  const path = await import('node:path');
  const workflowPath = path.resolve(process.cwd(), '.github/workflows/sync-pricing.yml');
  const content = fs.readFileSync(workflowPath, 'utf8');

  assert.match(content, /^name:\s*Sync Model Pricing/m);
  assert.match(content, /schedule:/);
  assert.match(content, /cron:\s*'0 2 \* \* \*'/);
  assert.match(content, /workflow_dispatch:/);
  assert.match(content, /## Release notes zh-CN/);
  assert.match(content, /## Release notes en-US/);
  // Ensure no heredoc lines reset to column 1 inside run block
  const lines = content.split('\n');
  let inRunBlock = false;
  let runIndent = 0;
  for (let i = 0; i < lines.length; i++) {
    const line = lines[i];
    if (/^\s*run:\s*\|/.test(line)) {
      inRunBlock = true;
      runIndent = line.search(/\S/);
      continue;
    }
    if (inRunBlock) {
      if (line.trim() === '') continue;
      const indent = line.search(/\S/);
      if (indent <= runIndent) {
        // Reached next YAML key
        inRunBlock = false;
      }
    }
  }
});
