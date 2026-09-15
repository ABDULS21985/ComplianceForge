declare module 'jest-axe' {
  interface AxeResult {
    violations: unknown[];
  }

  export function axe(html: Element | string): Promise<AxeResult>;
}
