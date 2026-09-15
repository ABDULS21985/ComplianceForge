'use client';

import { useSyncExternalStore } from 'react';

function subscribeToConnectivity(onStoreChange: () => void) {
  window.addEventListener('online', onStoreChange);
  window.addEventListener('offline', onStoreChange);

  return () => {
    window.removeEventListener('online', onStoreChange);
    window.removeEventListener('offline', onStoreChange);
  };
}

function readConnectivity() {
  return navigator.onLine;
}

function readServerConnectivity() {
  return true;
}

/**
 * Tracks browser connectivity without creating an effect-driven copy of global state.
 * A positive value is only a transport hint; requests can still fail while online.
 */
export function useOnlineStatus() {
  return useSyncExternalStore(
    subscribeToConnectivity,
    readConnectivity,
    readServerConnectivity,
  );
}
