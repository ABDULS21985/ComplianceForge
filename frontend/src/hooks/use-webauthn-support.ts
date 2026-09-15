'use client';

import { useSyncExternalStore } from 'react';

import { webAuthnSupported } from '@/lib/identity';

const subscribe = () => () => undefined;
const getServerSnapshot = () => false;

/** Hydration-safe browser capability check; WebAuthn support has no change event. */
export function useWebAuthnSupport(): boolean {
  return useSyncExternalStore(subscribe, webAuthnSupported, getServerSnapshot);
}
