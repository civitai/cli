// Every fetch here targets a legitimately-REST endpoint that has NO host bridge.
// None of these may be flagged — flagging them would false-fail a correct app.

export async function models() {
  return fetch('/api/v1/blocks/models?limit=20'); // public catalog read
}

export async function images() {
  return fetch('/api/v1/blocks/images'); // public catalog read
}

export async function viewer(base: string, token: string) {
  // Viewer self-read: the useViewer() bridge exists but this route stays live
  // until the hook publishes and consumers migrate; scaffolds use it in dev.
  return fetch(`${base}/api/v1/blocks/me`, { headers: { authorization: `Bearer ${token}` } });
}

export async function devToken(base: string) {
  // Headless dev-token mint — a CLI/dev path, not a runtime block surface.
  return fetch(`${base}/api/v1/blocks/dev-token`, { method: 'POST' });
}
