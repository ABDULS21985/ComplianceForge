// @vitest-environment node

import { describe, expect, it } from 'vitest';
import { join } from 'node:path';
import { readFileSync } from 'node:fs';

type RGB = [number, number, number];

function rgb(hex: string): RGB {
  const normalized = hex.replace('#', '');
  return [0, 2, 4].map((offset) => Number.parseInt(normalized.slice(offset, offset + 2), 16)) as RGB;
}

function luminance([red, green, blue]: RGB) {
  const channels = [red, green, blue].map((channel) => {
    const value = channel / 255;
    return value <= 0.04045 ? value / 12.92 : ((value + 0.055) / 1.055) ** 2.4;
  });
  return 0.2126 * channels[0] + 0.7152 * channels[1] + 0.0722 * channels[2];
}

function contrast(foreground: string, background: string) {
  const light = Math.max(luminance(rgb(foreground)), luminance(rgb(background)));
  const dark = Math.min(luminance(rgb(foreground)), luminance(rgb(background)));
  return (light + 0.05) / (dark + 0.05);
}

describe('shared accessibility style contract', () => {
  it.each([
    ['white on indigo action', '#ffffff', '#4338ca'],
    ['white on green action', '#ffffff', '#15803d'],
    ['body text on white', '#111827', '#ffffff'],
    ['secondary text on white', '#4b5563', '#ffffff'],
    ['destructive text on light red', '#991b1b', '#fef2f2'],
  ])('%s meets 4.5:1 for normal text', (_name, foreground, background) => {
    expect(contrast(foreground, background)).toBeGreaterThanOrEqual(4.5);
  });

  it('provides global reduced-motion, visible-focus, coarse-pointer, and forced-colour fallbacks', () => {
    const css = readFileSync(join(process.cwd(), 'src/app/globals.css'), 'utf8');
    expect(css).toContain('@media (prefers-reduced-motion: reduce)');
    expect(css).toContain('animation-duration: 1ms !important');
    expect(css).toContain(':focus-visible');
    expect(css).toContain('outline: 2px solid hsl(var(--ring))');
    expect(css).toContain('@media (pointer: coarse)');
    expect(css).toContain('min-block-size: 44px');
    expect(css).toContain('@media (forced-colors: active)');
  });

  it('keeps shared overlays and tables structurally reflowable', () => {
    const dialog = readFileSync(join(process.cwd(), 'src/components/ui/dialog.tsx'), 'utf8');
    const table = readFileSync(join(process.cwd(), 'src/components/data/data-table.tsx'), 'utf8');
    expect(dialog).toContain('w-[calc(100%-2rem)]');
    expect(dialog).toContain('max-h-[calc(100dvh-2rem)]');
    expect(dialog).toContain('overflow-y-auto');
    expect(table).toContain('overflow-x-auto');
    expect(table).toContain('sm:flex-row');
  });
});
