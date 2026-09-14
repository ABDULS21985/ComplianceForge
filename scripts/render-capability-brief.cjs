const fs = require('fs');
const path = require('path');
const { pathToFileURL } = require('url');
const { chromium } = require('../frontend/node_modules/playwright');

const root = path.resolve(__dirname, '..');
const input = path.resolve(
  process.argv[2] || path.join(root, 'docs', 'ComplianceForge-capability-brief.html'),
);
const output = path.resolve(
  process.argv[3] || path.join(root, 'docs', 'ComplianceForge-capability-brief.pdf'),
);

if (!fs.existsSync(input)) {
  throw new Error(`Source document not found: ${input}`);
}

const chromeCandidates = [
  process.env.CHROME_PATH,
  '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome',
  '/usr/bin/google-chrome',
  '/usr/bin/chromium',
].filter(Boolean);

const executablePath = chromeCandidates.find((candidate) => fs.existsSync(candidate));

(async () => {
  const browser = await chromium.launch({
    headless: true,
    ...(executablePath ? { executablePath } : {}),
  });

  try {
    const page = await browser.newPage({ viewport: { width: 816, height: 1056 } });
    await page.goto(pathToFileURL(input).href, { waitUntil: 'networkidle' });
    await page.evaluate(() => document.fonts.ready);
    await page.pdf({
      path: output,
      format: 'Letter',
      printBackground: true,
      preferCSSPageSize: true,
      displayHeaderFooter: false,
      tagged: true,
      margin: { top: 0, right: 0, bottom: 0, left: 0 },
    });
  } finally {
    await browser.close();
  }

  process.stdout.write(`${output}\n`);
})().catch((error) => {
  console.error(error);
  process.exitCode = 1;
});
