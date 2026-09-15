import { access, cp } from 'node:fs/promises';
import { join } from 'node:path';
import { pathToFileURL } from 'node:url';

const frontendRoot = process.cwd();
const standaloneRoot = join(frontendRoot, '.next/standalone');

async function copyIfPresent(source, destination) {
  try {
    await access(source);
  } catch (error) {
    if (error.code === 'ENOENT') return;
    throw error;
  }
  await cp(source, destination, { recursive: true });
}

// Match the production image's standalone asset layout. These destinations are
// generated build artifacts, never source/configuration files.
await copyIfPresent(join(frontendRoot, '.next/static'), join(standaloneRoot, '.next/static'));
await copyIfPresent(join(frontendRoot, 'public'), join(standaloneRoot, 'public'));
process.env.PORT = '3100';
process.env.HOSTNAME = '127.0.0.1';
await import(pathToFileURL(join(standaloneRoot, 'server.js')).href);
