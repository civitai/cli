// ANTI-PATTERN: fetches the REMOVED /api/v1/blocks/buzz REST route instead of
// reading buzz through the host bridge (useBuzzBalance()). This is the exact
// dead-endpoint that broke playable-collections.
export async function getBuzz(base: string, token: string) {
  const res = await fetch(`${base}/api/v1/blocks/buzz`, {
    headers: { authorization: `Bearer ${token}` },
  });
  return res.json();
}
