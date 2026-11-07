import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const DEFAULT_UPSTREAM_URL =
  'https://raw.githubusercontent.com/BerriAI/litellm/main/model_prices_and_context_window.json';

const RELEVANT_PREFIXES = [
  'claude-',
  'gemini-',
  'gpt-',
  'o1',
  'o3',
  'deepseek',
  'codex',
  'qwen',
];

export function roundPrice(num) {
  if (!num || Number.isNaN(num)) return 0;
  // Round to up to 4 decimal places, avoiding floating point artifacts
  return Math.round((num + Number.EPSILON) * 10000) / 10000;
}

export function normalizeModelKey(rawKey) {
  let key = rawKey.trim();
  // Strip provider prefixes e.g. "anthropic/claude-3-7-sonnet" -> "claude-3-7-sonnet"
  if (key.includes('/')) {
    const parts = key.split('/');
    key = parts[parts.length - 1];
  }
  // Strip version tags e.g. "claude-3-7-sonnet:0" -> "claude-3-7-sonnet"
  if (key.includes(':')) {
    key = key.split(':')[0];
  }
  return key.toLowerCase();
}

export function isRelevantModel(rawKey, entry) {
  if (!entry || typeof entry !== 'object') return false;
  const mode = entry.mode || 'chat';
  if (['embedding', 'rerank', 'audio_transcription', 'image_generation', 'moderation'].includes(mode)) {
    return false;
  }

  const key = normalizeModelKey(rawKey);
  const isPrefixed = RELEVANT_PREFIXES.some((prefix) => key.startsWith(prefix));
  if (!isPrefixed) return false;

  const inCost = entry.input_cost_per_token || 0;
  const outCost = entry.output_cost_per_token || 0;
  return inCost > 0 || outCost > 0;
}

export function convertUpstreamPrice(entry) {
  const inPrice = roundPrice((entry.input_cost_per_token || 0) * 1e6);
  const outPrice = roundPrice((entry.output_cost_per_token || 0) * 1e6);

  // Default cache read to 10% of input price if not explicitly provided
  let cachedPrice = 0;
  if (typeof entry.cache_read_input_token_cost === 'number') {
    cachedPrice = roundPrice(entry.cache_read_input_token_cost * 1e6);
  } else if (inPrice > 0) {
    cachedPrice = roundPrice(inPrice * 0.1);
  }

  const cacheCreationPrice =
    typeof entry.cache_creation_input_token_cost === 'number'
      ? roundPrice(entry.cache_creation_input_token_cost * 1e6)
      : 0;

  return {
    input_price_per_m: inPrice,
    cached_price_per_m: cachedPrice,
    cache_creation_price_per_m: cacheCreationPrice,
    output_price_per_m: outPrice,
  };
}

export function mergePricingTables(currentTable, upstreamData, options = {}) {
  const currentModels = currentTable?.models || {};
  const mergedModels = { ...currentModels };

  let addedCount = 0;
  let updatedCount = 0;

  for (const [rawKey, entry] of Object.entries(upstreamData)) {
    if (!isRelevantModel(rawKey, entry)) continue;

    const modelKey = normalizeModelKey(rawKey);
    const newPrice = convertUpstreamPrice(entry);

    if (!mergedModels[modelKey]) {
      mergedModels[modelKey] = newPrice;
      addedCount++;
    } else if (options.overwriteExisting) {
      const existing = mergedModels[modelKey];
      if (newPrice.cache_creation_price_per_m === 0 && existing.cache_creation_price_per_m > 0) {
        newPrice.cache_creation_price_per_m = existing.cache_creation_price_per_m;
      }
      const differs =
        Math.abs(existing.input_price_per_m - newPrice.input_price_per_m) > 0.0001 ||
        Math.abs(existing.output_price_per_m - newPrice.output_price_per_m) > 0.0001;
      if (differs) {
        mergedModels[modelKey] = { ...existing, ...newPrice };
        updatedCount++;
      }
    }
  }

  return {
    merged: { models: mergedModels },
    addedCount,
    updatedCount,
  };
}

export function formatPricingTableJSON(table) {
  const sortedModels = {};
  const keys = Object.keys(table.models || {}).sort();
  for (const k of keys) {
    sortedModels[k] = table.models[k];
  }
  return JSON.stringify({ models: sortedModels }, null, 2) + '\n';
}

export async function runSync(options = {}) {
  const upstreamUrl = options.upstreamUrl || process.env.PRICING_UPSTREAM_URL || DEFAULT_UPSTREAM_URL;
  const pricingPath =
    options.pricingPath ||
    path.resolve(path.dirname(fileURLToPath(import.meta.url)), '../cmd/dashboard/pricing_table.json');

  console.log(`Fetching upstream pricing from: ${upstreamUrl}`);
  const response = await fetch(upstreamUrl);
  if (!response.ok) {
    throw new Error(`Failed to fetch upstream pricing: HTTP ${response.status}`);
  }
  const upstreamData = await response.json();

  let currentTable = { models: {} };
  if (fs.existsSync(pricingPath)) {
    currentTable = JSON.parse(fs.readFileSync(pricingPath, 'utf8'));
  }

  const { merged, addedCount, updatedCount } = mergePricingTables(currentTable, upstreamData);
  console.log(`Sync results: ${addedCount} models added, ${updatedCount} models updated.`);

  if (addedCount > 0 || updatedCount > 0) {
    const outputJSON = formatPricingTableJSON(merged);
    fs.writeFileSync(pricingPath, outputJSON, 'utf8');
    console.log(`Updated pricing table written to: ${pricingPath}`);
    return { changed: true, addedCount, updatedCount };
  } else {
    console.log('Pricing table is already up-to-date.');
    return { changed: false, addedCount: 0, updatedCount: 0 };
  }
}

// CLI execution
if (process.argv[1] && fileURLToPath(import.meta.url) === path.resolve(process.argv[1])) {
  runSync().catch((err) => {
    console.error('Sync failed:', err);
    process.exit(1);
  });
}
