#!/usr/bin/env node
/**
 * Copyright 2025 Alibaba Group Holding Ltd.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */


/**
 * Generate spec-inline.js from sandbox-lifecycle.yml
 *
 * Usage:
 *   node scripts/generate-spec.js
 *
 * This script:
 * 1. Reads specs/sandbox-lifecycle.yml
 * 2. Escapes backticks
 * 3. Wraps in JavaScript template literal
 * 4. Writes to docs/spec-inline.js
 */

const fs = require('fs');
const path = require('path');

// Find project root
function findProjectRoot() {
  let dir = __dirname;
  while (dir !== path.dirname(dir)) {
    if (fs.existsSync(path.join(dir, 'specs', 'sandbox-lifecycle.yml'))) {
      return dir;
    }
    dir = path.dirname(dir);
  }
  throw new Error('Could not find project root (sandbox-lifecycle.yml not found)');
}

function main() {
  try {
    const projectRoot = findProjectRoot();
    const yamlPath = path.join(projectRoot, 'specs', 'sandbox-lifecycle.yml');
    const outputPath = path.join(projectRoot, 'docs', 'spec-inline.js');

    // Validate input file exists
    if (!fs.existsSync(yamlPath)) {
      throw new Error(`YAML file not found: ${yamlPath}`);
    }

    console.log('📝 Generating spec-inline.js...');
    console.log(`   Input:  ${yamlPath}`);
    console.log(`   Output: ${outputPath}`);

    // Read YAML
    const yamlContent = fs.readFileSync(yamlPath, 'utf-8');
    const yamlSize = Math.round(yamlContent.length / 1024);

    // Escape backticks for template literal
    const escapedYaml = yamlContent.replace(/`/g, '\\`');

    // Generate JavaScript
    const jsContent = `const OPENAPI_SPEC_YAML = \`${escapedYaml}\`;`;
    const jsSize = Math.round(jsContent.length / 1024);

    // Write output
    fs.writeFileSync(outputPath, jsContent, 'utf-8');

    console.log(`\n✅ Successfully generated spec-inline.js`);
    console.log(`   YAML size: ${yamlSize} KB`);
    console.log(`   JS size:   ${jsSize} KB`);
    console.log(`   Compression ratio: ${((jsSize / yamlSize) * 100).toFixed(1)}%`);

    // Verify
    const generated = fs.readFileSync(outputPath, 'utf-8');
    if (generated.startsWith('const OPENAPI_SPEC_YAML = `')) {
      console.log(`\n✅ File validated successfully`);
      process.exit(0);
    } else {
      throw new Error('Generated file validation failed');
    }
  } catch (error) {
    console.error(`\n❌ Error: ${error.message}`);
    console.error(error.stack);
    process.exit(1);
  }
}

// Run
main();
